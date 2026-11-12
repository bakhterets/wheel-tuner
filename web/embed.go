// Package web embeds the static frontend (HTML, CSS, ES modules).
package web

import (
	"embed"
	"io/fs"
)

//go:embed static
var files embed.FS

// Static returns the frontend file tree rooted at the static directory.
func Static() (fs.FS, error) {
	return fs.Sub(files, "static")
}
