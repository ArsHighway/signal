package db

import "embed"

// FS keeps the versioned goose migrations in the server binary.
//
//go:embed migrations/*.sql
var FS embed.FS
