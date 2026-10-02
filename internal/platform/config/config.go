// Package config содержит только чтение настроек процесса, не бизнес-правила.
package config

import "os"

func Env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
