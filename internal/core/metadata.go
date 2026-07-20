package core

import (
	"encoding/json"

	"github.com/joseph0x45/nuage/internal/index"
)

// fileMeta is the disaster-recovery metadata mirrored into each upload's
// Telegram message caption. Size, MessageID, ChannelID, and UploadedAt
// aren't included — they're recovered directly from the message itself
// (Document.Size, the message id, the channel, and the message's send
// date) when reindexing, so the caption only needs to carry what Telegram
// doesn't already know.
type fileMeta struct {
	Path     string `json:"path"`
	Filename string `json:"filename"`
	Owner    string `json:"owner"`
	Hash     string `json:"hash"`
}

func encodeFileMeta(rec *index.Record) (string, error) {
	b, err := json.Marshal(fileMeta{
		Path:     rec.Path,
		Filename: rec.Filename,
		Owner:    rec.Owner,
		Hash:     rec.Hash,
	})
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// decodeFileMeta parses a message caption as fileMeta. ok is false for
// captions that aren't recognizable Nuage metadata (e.g. empty, or a
// message posted some other way) — Reindex skips those rather than erroring
// the whole scan.
func decodeFileMeta(caption string) (meta fileMeta, ok bool) {
	if err := json.Unmarshal([]byte(caption), &meta); err != nil {
		return fileMeta{}, false
	}
	if meta.Filename == "" || meta.Hash == "" {
		return fileMeta{}, false
	}
	return meta, true
}
