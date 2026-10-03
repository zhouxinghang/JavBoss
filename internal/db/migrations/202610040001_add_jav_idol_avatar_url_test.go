package migrations

import (
	"context"
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func TestAddJavIdolAvatarURLDefaultsExistingRows(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE jav_idol (id integer PRIMARY KEY, name text); INSERT INTO jav_idol (id, name) VALUES (1, '三上悠亜')`); err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for i := 0; i < 2; i++ {
		if err := addJavIdolAvatarURL(context.Background(), tx); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	var name string
	var avatarURL string
	if err := db.QueryRow(`SELECT name, avatar_url FROM jav_idol WHERE id = 1`).Scan(&name, &avatarURL); err != nil {
		t.Fatal(err)
	}
	if name != "三上悠亜" || avatarURL != "" {
		t.Fatalf("row after migration: name=%q avatar_url=%q", name, avatarURL)
	}
}
