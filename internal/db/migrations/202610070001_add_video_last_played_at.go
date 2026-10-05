package migrations

import (
	"context"
	"database/sql"

	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddNamedMigrationContext("202610070001_add_video_last_played_at.go", addVideoLastPlayedAt, irreversibleMigration)
}

// addVideoLastPlayedAt records when a video was last played so the library can
// offer a recently watched list ordered by actual watch time.
func addVideoLastPlayedAt(ctx context.Context, tx *sql.Tx) error {
	return addColumnIfMissing(ctx, tx, "video", "last_played_at", "datetime")
}
