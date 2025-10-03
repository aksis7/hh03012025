package download

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
)

// DeriveFileName определяет имя файла для сохранения.
// Использует последний сегмент пути URL, если он есть; иначе
// генерирует имя вида "file_<индекс>". Параметры после "?" отбрасываются.
func DeriveFileName(rawURL string, index int) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Path == "" || u.Path == "/" {
		return fmt.Sprintf("file_%d", index)
	}

	segments := strings.Split(u.Path, "/")
	name := segments[len(segments)-1]

	if i := strings.Index(name, "?"); i != -1 {
		name = name[:i]
	}
	if name == "" {
		name = fmt.Sprintf("file_%d", index)
	}
	return name
}

// DownloadWithContext скачивает файл по заданному URL и записывает его в dest.
// Скачивание отменяется через ctx. Каталоги для dest должны быть созданы
// заранее. Запись ведётся во временный файл и затем атомарно переименовывается
// в конечное имя, чтобы избежать частичных файлов при сбоях.
func DownloadWithContext(ctx context.Context, fileURL, dest string) error {
	// Создаем запрос с контекстом для отмены
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	if err != nil {
		return err
	}

	// Используем клиент без фиксированного таймаута; полагаемся на контекст для отмены
	client := &http.Client{Timeout: 0}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Проверяем статус ответа, если он не в диапазоне 2xx — ошибка
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("неправильный статус: %s", resp.Status)
	}

	// Создаем временный файл в той же директории
	tmp := dest + ".part"
	tmpFile, err := os.Create(tmp)
	if err != nil {
		return err
	}
	defer tmpFile.Close()

	// Копируем тело ответа в временный файл
	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		return err
	}

	// Обеспечиваем, чтобы данные были записаны в файл
	if err := tmpFile.Sync(); err != nil {
		return err
	}

	// Закрываем временный файл
	if err := tmpFile.Close(); err != nil {
		return err
	}

	// Переименовываем временный файл в целевой
	return os.Rename(tmp, dest)

}
