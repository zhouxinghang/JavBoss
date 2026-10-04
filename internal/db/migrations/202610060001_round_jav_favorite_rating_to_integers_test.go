package migrations

import (
	"context"
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func TestRoundJavFavoriteRatingToIntegers(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE jav (id integer PRIMARY KEY, favorite_rating real NOT NULL DEFAULT 0);
		INSERT INTO jav (id, favorite_rating) VALUES (1, 0.5), (2, 1.5), (3, 4.5), (4, 3), (5, 0)`); err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err := roundJavFavoriteRatingToIntegers(context.Background(), tx); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	want := map[int64]float64{1: 1, 2: 2, 3: 5, 4: 3, 5: 0}
	for id, expected := range want {
		var got float64
		if err := db.QueryRow(`SELECT favorite_rating FROM jav WHERE id = ?`, id).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != expected {
			t.Fatalf("id %d favorite_rating = %v, want %v", id, got, expected)
		}
	}
}
