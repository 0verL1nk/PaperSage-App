// Package web exposes the frontend build output embedded at compile time.
// The build pipeline copies frontend/dist into web/assets before go build;
// in local dev the directory only holds a placeholder.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:assets
var assets embed.FS

// Dist returns the embedded frontend filesystem rooted at the dist top level.
func Dist() (fs.FS, error) {
	return fs.Sub(assets, "assets")
}
