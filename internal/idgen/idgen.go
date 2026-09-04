// Package idgen генерирует короткие случайные ID (crypto/rand, без зависимостей).
package idgen

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// New возвращает hex-строку из 8 случайных байт.
func New() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand практически не возвращает ошибок; при сбое — паника
		panic(fmt.Sprintf("idgen: crypto/rand: %v", err))
	}
	return hex.EncodeToString(b[:])
}
