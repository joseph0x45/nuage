package web

import (
	"embed"
	"io/fs"
)

//go:embed static/index.html static/app.js static/style.css
var staticFiles embed.FS

func staticFS() fs.FS {
	sub, err := fs.Sub(staticFiles, "static")
	if err != nil {
		// static/ is embedded above; this can only fail if the embed
		// directive itself is wrong, which build would already have caught.
		panic(err)
	}
	return sub
}
