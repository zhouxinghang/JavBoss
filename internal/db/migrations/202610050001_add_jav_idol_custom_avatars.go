package migrations

import (
	"context"
	"database/sql"

	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddNamedMigrationContext("202610050001_add_jav_idol_custom_avatars.go", addJavIdolCustomAvatars, irreversibleMigration)
}

func addJavIdolCustomAvatars(ctx context.Context, tx *sql.Tx) error {
	return addColumnIfMissing(ctx, tx, "jav_idol", "custom_avatars", `text NOT NULL DEFAULT "[]"`)
}
