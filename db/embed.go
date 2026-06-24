// Package db embeds the SQL migration files so the server can apply them on
// startup without shipping a separate migrations directory.
package db

import "embed"

//go:embed migrations/*.sql
var Migrations embed.FS
