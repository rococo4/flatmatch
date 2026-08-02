package migrations

import "embed"

// FS holds the goose migration files, newest last.
//
//go:embed *.sql
var FS embed.FS
