package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"hh03012025/internal/api"
	"hh03012025/internal/manager"
)

// main — точка входа сервиса загрузки файлов. Здесь настраивается
// менеджер задач, загружается состояние из снапшота, запускаются воркеры и
// периодическая запись состояния, а также поднимается HTTP‑сервер
// с эндпоинтами для создания задач и получения статуса. Реализовано
// корректное завершение: при получении сигнала ожидание завершения
// текущих загрузок и сохранение состояния.
func main() {
	// Настройки по умолчанию. Их можно заменить переменными окружения или флагами.
	downloadDir := "downloads"
	snapshotFile := "tasks_snapshot.json"
	workerCount := 5
	jobQueueSize := 100

	// Создаём менеджер с буферизированной очередью заданий.
	mgr := manager.NewManager(jobQueueSize)
	// Корневой контекст для воркеров и задачи снапшота. Отмена
	// распространится на все горутины, использующие этот ctx.
	ctx, cancel := context.WithCancel(context.Background())

	// Восстанавливаем состояние из снапшота и ставим незавершённые файлы в очередь.
	mgr.LoadFromSnapshot(snapshotFile, downloadDir)
	// Запускаем воркеры для обработки очереди скачиваний.
	mgr.StartWorkers(ctx, workerCount, downloadDir)
	// Периодически сохраняем состояние задач на диск.
	go mgr.SnapshotLoop(ctx, snapshotFile, 15*time.Second)

	// Настраиваем маршруты HTTP и мидлвар.
	mux := http.NewServeMux()
	mux.HandleFunc("/tasks", api.NewCreateTaskHandler(mgr))
	mux.HandleFunc("/tasks/", api.NewGetTaskHandler(mgr))
	handler := api.WithCORS(mux)
	srv := &http.Server{Addr: ":8080", Handler: handler}

	// Обработка сигналов для корректного завершения.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("получен сигнал завершения, начинаем корректное завершение")
		// Прекращаем приём новых соединений.
		if err := srv.Shutdown(context.Background()); err != nil {
			log.Printf("ошибка при остановке сервера: %v", err)
		}
		// Отменяем контекст, чтобы завершить воркеры и запись снапшота.
		cancel()
		// Ждём завершения активных загрузок.
		log.Println("ожидаем завершения активных загрузок...")
		mgr.Wait()
		log.Println("загрузки завершены, выходим")
	}()

	log.Printf("запуск сервера на %s", srv.Addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("ошибка сервера: %v", err)
	}
}
