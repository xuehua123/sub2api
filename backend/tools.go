//go:build tools

// Package tools pins command-line tools used during development and code
// generation. Listing them here (rather than only invoking `go run`) keeps
// their transitive dependencies in go.mod/go.sum so `go mod tidy` does not
// drop them and `go generate ./cmd/server` keeps working.
package tools

import (
	_ "github.com/google/wire/cmd/wire"
)
