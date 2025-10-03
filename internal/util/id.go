package util

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// GenerateID генерирует случайный 16‑байтовый hex‑идентификатор.
// Если нет доступа к источнику случайных данных, используется текущее время.
// Используется для присвоения уникальных идентификаторов задачам.
func GenerateID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}
