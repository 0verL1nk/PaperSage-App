package assets

import "embed"

// Dist is the built frontend. `make desktop-build` copies frontend/dist here
// before compiling; the directory is gitignored so the Go build always embeds
// the current frontend output.
//
//go:embed all:dist
var Dist embed.FS
