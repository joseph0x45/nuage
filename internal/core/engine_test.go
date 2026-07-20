package core

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gotd/td/tg"
)

func TestHashFileNotFound(t *testing.T) {
	if _, _, err := hashFile(filepath.Join(t.TempDir(), "does-not-exist")); err == nil {
		t.Fatal("hashFile on missing path: expected error, got nil")
	}
}

func TestHashFileKnownContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sample.txt")
	content := []byte("nuage")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	hash, size, err := hashFile(path)
	if err != nil {
		t.Fatalf("hashFile: unexpected error %v", err)
	}
	// sha256("nuage")
	const want = "79947181bfe3a9fc636b289e4dfb758036002b9b458e369c4ca0ad8e112f131c"
	if hash != want {
		t.Errorf("hash = %q, want %q", hash, want)
	}
	if size != int64(len(content)) {
		t.Errorf("size = %d, want %d", size, len(content))
	}
}

func TestMessageIDWrongUpdateType(t *testing.T) {
	if _, err := messageID(&tg.UpdatesTooLong{}); err == nil {
		t.Fatal("messageID on non-*tg.Updates: expected error, got nil")
	}
}

func TestMessageIDNoNewChannelMessage(t *testing.T) {
	updates := &tg.Updates{
		Updates: []tg.UpdateClass{&tg.UpdateReadHistoryInbox{}},
	}
	if _, err := messageID(updates); err == nil {
		t.Fatal("messageID with no UpdateNewChannelMessage: expected error, got nil")
	}
}

func TestMessageIDSuccess(t *testing.T) {
	updates := &tg.Updates{
		Updates: []tg.UpdateClass{
			&tg.UpdateNewChannelMessage{
				Message: &tg.Message{ID: 42},
			},
		},
	}
	id, err := messageID(updates)
	if err != nil {
		t.Fatalf("messageID: unexpected error %v", err)
	}
	if id != 42 {
		t.Errorf("id = %d, want 42", id)
	}
}
