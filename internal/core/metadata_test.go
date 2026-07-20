package core

import (
	"testing"

	"github.com/joseph0x45/nuage/internal/index"
)

func TestEncodeDecodeFileMetaRoundTrip(t *testing.T) {
	rec := &index.Record{Path: "a.txt", Filename: "a.txt", Owner: "joseph", Hash: "abc123"}

	caption, err := encodeFileMeta(rec)
	if err != nil {
		t.Fatalf("encodeFileMeta: %v", err)
	}

	meta, ok := decodeFileMeta(caption)
	if !ok {
		t.Fatalf("decodeFileMeta(%q): expected ok=true", caption)
	}
	if meta.Path != rec.Path || meta.Filename != rec.Filename || meta.Owner != rec.Owner || meta.Hash != rec.Hash {
		t.Errorf("decoded meta = %+v, want %+v", meta, rec)
	}
}

func TestDecodeFileMetaRejectsUnrecognized(t *testing.T) {
	for _, caption := range []string{
		"",
		"not json",
		"{}",
		`{"filename":"a.txt"}`, // missing hash
		`{"hash":"abc123"}`,    // missing filename
		"just a regular caption a user might type",
	} {
		if _, ok := decodeFileMeta(caption); ok {
			t.Errorf("decodeFileMeta(%q): expected ok=false", caption)
		}
	}
}
