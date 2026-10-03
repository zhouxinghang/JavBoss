package migrations

import (
	"context"
	"database/sql"

	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddNamedMigrationContext("202610040001_add_jav_idol_avatar_url.go", addJavIdolAvatarURL, irreversibleMigration)
}

func addJavIdolAvatarURL(ctx context.Context, tx *sql.Tx) error {
	return addColumnIfMissing(ctx, tx, "jav_idol", "avatar_url", `text NOT NULL DEFAULT ""`)
}
