package model

import "time"

// FileState описывает состояние отдельного файла в задаче.
// Файл может находиться в одном из состояний: "pending" (ожидание),
// "in‑progress" (скачивание в процессе), "completed" (скачан) или "error" (ошибка).
// Поле Error заполняется, если при скачивании произошла ошибка.
type FileState struct {
	URL    string `json:"url"`             // original URL to download
	Status string `json:"status"`          // one of: pending, in‑progress, completed, error
	Error  string `json:"error,omitempty"` // description of any failure
}

// Task represents a download task submitted by the user.
// A task contains multiple files and overall status information.
// Status can be: "pending", "in‑progress", "completed", "completed_with_errors".
// Task описывает задачу скачивания. Содержит список файлов (Files), общий статус
// (Status) и временные метки создания и последнего обновления. Возможные
// значения Status: "pending" (ожидает), "in‑progress" (в процессе),
// "completed" (все файлы скачаны), "completed_with_errors" (скачано, но были ошибки).
type Task struct {
	ID        string      `json:"id"`         // уникальный идентификатор
	Files     []FileState `json:"files"`      // список файлов и их состояния
	Status    string      `json:"status"`     // общий статус задачи
	CreatedAt time.Time   `json:"created_at"` // время создания
	UpdatedAt time.Time   `json:"updated_at"` // время последнего обновления
}
