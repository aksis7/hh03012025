package manager

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"hh03012025/internal/download"
	"hh03012025/internal/model"
	"hh03012025/internal/util"
)

// Job — элемент очереди, определяющий конкретный файл в задаче для скачивания.
// Поле FileIndex соответствует индексу в срезе Task.Files.
type Job struct {
	TaskID    string
	FileIndex int
}

// Manager управляет задачами: принимает новые, планирует скачивание файлов,
// запускает рабочие воркеры, следит за состоянием и сохраняет/восстанавливает
// состояние из снапшотов. Допускает параллельный доступ.
type Manager struct {
	tasks    map[string]*model.Task
	mu       sync.RWMutex
	jobs     chan Job
	wg       sync.WaitGroup
	draining bool
}

// NewManager создаёт и возвращает менеджер. Параметр queueSize задаёт
// ёмкость буферизированной очереди заданий (jobs).
func NewManager(queueSize int) *Manager {
	return &Manager{
		tasks: make(map[string]*model.Task),
		jobs:  make(chan Job, queueSize),
	}
}

// AddTask создаёт новую задачу по списку URL, присваивает ей уникальный
// идентификатор и ставит все файлы в очередь на скачивание. Если менеджер
// находится в режиме draining (при остановке), задания будут поставлены
// только после перезапуска. В поле Status возвращаемой задачи можно понять,
// были ли начаты скачивания.
func (m *Manager) AddTask(urls []string) (*model.Task, error) {
	if len(urls) == 0 {
		return nil, errors.New("task must contain at least one URL")
	}
	id := util.GenerateID()
	now := time.Now().UTC()
	files := make([]model.FileState, len(urls))
	for i, u := range urls {
		files[i] = model.FileState{URL: u, Status: "pending"}
	}
	t := &model.Task{
		ID:        id,
		Files:     files,
		Status:    "pending",
		CreatedAt: now,
		UpdatedAt: now,
	}
	m.mu.Lock()
	m.tasks[id] = t
	m.mu.Unlock()
	if !m.draining {
		for idx := range files {
			m.enqueueJob(t.ID, idx)
		}
	}
	return t, nil
}

// enqueueJob помещает указанный файл в очередь на скачивание и помечает его
// состояние как pending (ожидание), если это необходимо.
func (m *Manager) enqueueJob(taskID string, fileIndex int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	task, ok := m.tasks[taskID]
	if !ok || fileIndex < 0 || fileIndex >= len(task.Files) {
		return
	}
	if task.Files[fileIndex].Status != "completed" {
		task.Files[fileIndex].Status = "pending"
		task.UpdatedAt = time.Now().UTC()
		m.jobs <- Job{TaskID: taskID, FileIndex: fileIndex}
	}
}

// GetTask возвращает копию задачи по ID, если она существует. Возвращает вторым
// значением false, если задача неизвестна.
func (m *Manager) GetTask(id string) (*model.Task, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.tasks[id]
	if !ok {
		return nil, false
	}
	// return a deep copy to avoid exposing internal pointers
	copyTask := *t
	copyTask.Files = make([]model.FileState, len(t.Files))
	copy(copyTask.Files, t.Files)
	return &copyTask, true
}

// StartWorkers запускает n воркеров, которые читают из канала jobs и скачивают
// файлы, пока контекст ctx не будет отменён. Воркеры учитываются в wait group,
// которая увеличивается при начале скачивания и уменьшается по завершению.
func (m *Manager) StartWorkers(ctx context.Context, n int, downloadDir string) {
	for i := 0; i < n; i++ {
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case job := <-m.jobs:
					m.processJob(ctx, job, downloadDir)
				}
			}
		}()
	}
}

// processJob выполняет скачивание конкретного файла. Он устанавливает статус
// файла "in‑progress", скачивает его, после чего помечает "completed" или
// "error". Также пересчитывает общий статус задачи после завершения всех
// файлов.
func (m *Manager) processJob(ctx context.Context, job Job, downloadDir string) {
	m.mu.Lock()
	task, ok := m.tasks[job.TaskID]
	if !ok {
		m.mu.Unlock()
		return
	}
	if job.FileIndex < 0 || job.FileIndex >= len(task.Files) || task.Files[job.FileIndex].Status == "completed" {
		m.mu.Unlock()
		return
	}

	task.Files[job.FileIndex].Status = "in‑progress"
	task.UpdatedAt = time.Now().UTC()
	task.Status = "in‑progress"
	m.mu.Unlock()

	m.wg.Add(1)
	defer m.wg.Done()

	fileURL := task.Files[job.FileIndex].URL
	dir := filepath.Join(downloadDir, job.TaskID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		m.updateFileState(job.TaskID, job.FileIndex, "error", err.Error())
		return
	}
	filename := download.DeriveFileName(fileURL, job.FileIndex)
	dest := filepath.Join(dir, filename)
	// download
	if err := download.DownloadWithContext(ctx, fileURL, dest); err != nil {
		m.updateFileState(job.TaskID, job.FileIndex, "error", err.Error())
	} else {
		m.updateFileState(job.TaskID, job.FileIndex, "completed", "")
	}
}

// updateFileState обновляет статус и сообщение об ошибке файла и
// пересчитывает общий статус задачи (учитывает наличие ошибок и завершение
// всех скачиваний).
func (m *Manager) updateFileState(taskID string, index int, status, errMsg string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	task, ok := m.tasks[taskID]
	if !ok || index < 0 || index >= len(task.Files) {
		return
	}
	task.Files[index].Status = status
	task.Files[index].Error = errMsg
	task.UpdatedAt = time.Now().UTC()
	//status
	allDone := true
	anyErrors := false
	for _, f := range task.Files {
		if f.Status != "completed" {
			allDone = false
		}
		if f.Status == "error" {
			anyErrors = true
		}
	}
	if allDone {
		if anyErrors {
			task.Status = "completed_with_errors"
		} else {
			task.Status = "completed"
		}
	} else {
		task.Status = "in‑progress"
	}
}

// SnapshotLoop периодически записывает текущее состояние задач в JSON‑файл.
// Работает до отмены контекста. Использует копию данных для серилизации,
// чтобы не блокировать обновления.
func (m *Manager) SnapshotLoop(ctx context.Context, filePath string, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			// perform a final snapshot before exit
			m.writeSnapshot(filePath)
			return
		case <-ticker.C:
			m.writeSnapshot(filePath)
		}
	}
}

// writeSnapshot сериализует все задачи в JSON и записывает их в указанный файл.
// Сначала создаёт временный файл, затем атомарно переименовывает его, чтобы
// избежать повреждения данных.
func (m *Manager) writeSnapshot(filePath string) {
	m.mu.RLock()
	// make a deep copy for serialization
	tasksCopy := make(map[string]*model.Task, len(m.tasks))
	for id, t := range m.tasks {
		taskCopy := *t
		taskCopy.Files = make([]model.FileState, len(t.Files))
		copy(taskCopy.Files, t.Files)
		tasksCopy[id] = &taskCopy
	}
	m.mu.RUnlock()
	data, err := json.MarshalIndent(tasksCopy, "", "  ")
	if err != nil {
		log.Printf("snapshot marshal error: %v", err)
		return
	}
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Printf("snapshot directory error: %v", err)
		return
	}
	tmp := filePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		log.Printf("snapshot write error: %v", err)
		return
	}
	if err := os.Rename(tmp, filePath); err != nil {
		log.Printf("snapshot rename error: %v", err)
		return
	}
}

// LoadFromSnapshot читает задачи из снапшота и загружает их в менеджер.
// Все файлы со статусами "pending", "in‑progress" или "error" помещаются
// обратно в очередь на скачивание. Вызывать до запуска воркеров.
func (m *Manager) LoadFromSnapshot(filePath, downloadDir string) {
	f, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		log.Printf("error opening snapshot: %v", err)
		return
	}
	defer f.Close()
	var tasks map[string]*model.Task
	if err := json.NewDecoder(f).Decode(&tasks); err != nil {
		log.Printf("snapshot decode error: %v", err)
		return
	}
	now := time.Now().UTC()
	m.mu.Lock()
	for id, task := range tasks {
		m.tasks[id] = task
		task.UpdatedAt = now
		// queue files not completed
		for idx, fs := range task.Files {
			if fs.Status != "completed" {
				task.Files[idx].Status = "pending"
				task.Files[idx].Error = ""
				m.jobs <- Job{TaskID: id, FileIndex: idx}
			}
		}
		task.Status = "in‑progress"
	}
	m.mu.Unlock()
}

// Wait блокируется до завершения всех активных скачиваний. Обычно вызывается
// во время корректного завершения работы, чтобы дождаться окончания задач.
func (m *Manager) Wait() {
	m.wg.Wait()
}
