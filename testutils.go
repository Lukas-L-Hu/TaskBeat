package main

import (
	"path/filepath"
	"testing"

	bolt "go.etcd.io/bbolt"
)

func TestDB(t *testing.T) *bolt.DB {
	t.Helper()

	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	db, err := bolt.Open(dbPath, 0600, nil)
	if err != nil {
		t.Fatalf("Failed to open BoltDB: %v", err)
	}

	err = db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte("Tasks"))
		return err
	})

	if err != nil {
		t.Fatalf("Failed to create bucket %v", err)
	}

	return db
}
