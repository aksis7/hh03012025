package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"time"

	"hh03012025/internal/manager"

	"hh03012025/internal/model"
)

// NewCreateTaskHandler возвращает HTTP‑обработчик для создания новой задачи.
// Ожидает JSON‑тело с полем "urls" — массивом ссылок. На успех отдаёт 202
// и идентификатор задачи. При ошибке возвращает 400 или 500.
func NewCreateTaskHandler(m *manager.Manager) http.HandlerFunc {
	type request struct {
		URLs []string `json:"urls"`
	}
	type response struct {
		TaskID string `json:"task_id"`
		Status string `json:"status"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		// trim whitespace and filter empty entries
		clean := make([]string, 0, len(req.URLs))
		for _, s := range req.URLs {
			s = strings.TrimSpace(s)
			if s != "" {
				clean = append(clean, s)
			}
		}
		task, err := m.AddTask(clean)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(response{TaskID: task.ID, Status: task.Status})
	}
}

// NewGetTaskHandler возвращает обработчик, который возвращает статус задачи по ID.
// Если задача не найдена, отвечает 404.
func NewGetTaskHandler(m *manager.Manager) http.HandlerFunc {
	type response struct {
		ID        string            `json:"id"`
		Status    string            `json:"status"`
		Completed int               `json:"completed"`
		Total     int               `json:"total"`
		Files     []model.FileState `json:"files"`
		CreatedAt time.Time         `json:"created_at"`
		UpdatedAt time.Time         `json:"updated_at"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		// expect /tasks/{id}
		parts := strings.Split(r.URL.Path, "/")
		if len(parts) != 3 || parts[2] == "" {
			http.Error(w, "task id missing", http.StatusBadRequest)
			return
		}
		id := parts[2]
		task, ok := m.GetTask(id)
		if !ok {
			http.NotFound(w, r)
			return
		}
		completed := 0
		for _, f := range task.Files {
			if f.Status == "completed" {
				completed++
			}
		}
		resp := response{
			ID:        task.ID,
			Status:    task.Status,
			Completed: completed,
			Total:     len(task.Files),
			Files:     task.Files,
			CreatedAt: task.CreatedAt,
			UpdatedAt: task.UpdatedAt,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

// WithCORS добавляет разрешающие CORS‑заголовки. Позволяет всем доменам
// отправлять GET, POST и OPTIONS запросы. Обёрнутый хендлер должен сам
// обрабатывать остальные методы.
func WithCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
