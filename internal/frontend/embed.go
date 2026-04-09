// Package frontend exposes the compiled React application as an embedded FS.
// The dist/ directory is populated at build time by copying frontend/dist/.
// In development, run `make build-frontend` first.
package frontend

import "embed"

//go:embed all:dist
var FS embed.FS
