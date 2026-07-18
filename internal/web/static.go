package web

import (
	"embed"
	"io/fs"
	"mime"
)

//go:embed static
var staticFiles embed.FS

func init() {
	// Go's built-in MIME table doesn't know this extension, so
	// http.FileServerFS would otherwise serve it as text/plain — some
	// browsers' PWA-installability checks are picky about that.
	if err := mime.AddExtensionType(".webmanifest", "application/manifest+json"); err != nil {
		panic(err)
	}
}

func staticFS() fs.FS {
	sub, err := fs.Sub(staticFiles, "static")
	if err != nil {
		// static/ is embedded above; this can only fail if the embed
		// directive itself is wrong, which build would already have caught.
		panic(err)
	}
	return sub
}
