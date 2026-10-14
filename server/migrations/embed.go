// Package migrations holds the SQL schema, embedded into the binary.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
