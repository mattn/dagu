package storage

import (
	"os"
	"path/filepath"
)

// Storage is a storage for flags.
type Storage struct {
	Dir string
}

// NewStorage creates a new storage.
func NewStorage(dir string) *Storage {
	os.MkdirAll(dir, 0755)
	return &Storage{
		Dir: dir,
	}
}

// Create creates the given file.
func (s *Storage) Create(file string) error {
	return os.WriteFile(filepath.Join(s.Dir, file), []byte{}, 0644)
}

// Exists returns true if the given file exists.
func (s *Storage) Exists(file string) bool {
	_, err := os.Stat(filepath.Join(s.Dir, file))
	return err == nil
}

// Delete deletes the given file.
func (s *Storage) Delete(file string) error {
	return os.Remove(filepath.Join(s.Dir, file))
}
