package config

import (
	"database/sql"
	"queuectl/internal/storage"
)

type Manager struct {
	store *storage.Storage
}

func NewManager(store *storage.Storage) *Manager {
	return &Manager{store: store}
}

// Set sets a configuration value.
func (m *Manager) Set(key, value string) error {
	query := `
		INSERT INTO config (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value
	`
	_, err := m.store.DB().Exec(query, key, value)
	return err
}

// Get gets a configuration value by key. Returns the default value if not found.
func (m *Manager) Get(key, defaultValue string) string {
	var value string
	err := m.store.DB().QueryRow(`SELECT value FROM config WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return defaultValue
	}
	if err != nil {
		return defaultValue
	}
	return value
}
