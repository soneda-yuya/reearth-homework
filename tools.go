//go:build tools

// Package tools tracks build-time dependencies so `go mod tidy` does not drop them.
// These tools are invoked via the Makefile, not imported in production code.
package tools

import (
	_ "golang.org/x/vuln/cmd/govulncheck"
)
