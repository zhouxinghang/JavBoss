package migrations

import (
	"context"
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func TestAddVideoLastPlayedAtPreservesExistingRows(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE video (id integer PRIMARY KEY, play_count integer NOT NULL DEFAULT 0); INSERT INTO video (id, play_count) VALUES (1, 7)`); err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for i := 0; i < 2; i++ {
		if err := addVideoLastPlayedAt(context.Background(), tx); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	var playCount int
	var lastPlayedAt sql.NullString
	if err := db.QueryRow(`SELECT play_count, last_played_at FROM video WHERE id = 1`).Scan(&playCount, &lastPlayedAt); err != nil {
		t.Fatal(err)
	}
	if playCount != 7 {
		t.Fatalf("play_count after migration = %d, want 7", playCount)
	}
	if lastPlayedAt.Valid {
		t.Fatalf("last_played_at should default to NULL for existing rows, got %q", lastPlayedAt.String)
	}
}
