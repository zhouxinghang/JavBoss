package migrations

import (
	"context"
	"database/sql"

	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddNamedMigrationContext(
		"202610060001_round_jav_favorite_rating_to_integers.go",
		roundJavFavoriteRatingToIntegers,
		irreversibleMigration,
	)
}

// roundJavFavoriteRatingToIntegers converts legacy half-star ratings into whole
// stars now that the UI only supports 1-5 in integer increments.
func roundJavFavoriteRatingToIntegers(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(
		ctx,
		`UPDATE jav SET favorite_rating = ROUND(favorite_rating) WHERE favorite_rating <> ROUND(favorite_rating)`,
	)
	return err
}
