package web

import "testing"

func TestContentTypeForFilename(t *testing.T) {
	cases := map[string]string{
		"clip.mp4":    "video/mp4",
		"clip.MOV":    "video/quicktime",
		"clip.webm":   "video/webm", // mime.TypeByExtension alone gets this wrong (audio/webm)
		"clip.mkv":    "video/x-matroska",
		"clip.avi":    "video/x-msvideo",
		"photo.png":   "image/png",
		"doc.pdf":     "application/pdf",
		"noext":       "application/octet-stream",
		"unknown.xyz": "application/octet-stream",
	}
	for name, want := range cases {
		if got := contentTypeForFilename(name); got != want {
			t.Errorf("contentTypeForFilename(%q) = %q, want %q", name, got, want)
		}
	}
}
