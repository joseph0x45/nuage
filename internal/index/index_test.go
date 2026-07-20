package index

import (
	"context"
	"errors"
	"testing"
	"time"
)

func openTest(t *testing.T) *Index {
	t.Helper()
	idx, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open test index: %v", err)
	}
	t.Cleanup(func() { idx.Close() })
	return idx
}

func TestGetNotFound(t *testing.T) {
	idx := openTest(t)
	if _, err := idx.Get(context.Background(), 999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get(999) error = %v, want ErrNotFound", err)
	}
}

func TestDeleteNotFound(t *testing.T) {
	idx := openTest(t)
	if err := idx.Delete(context.Background(), 999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Delete(999) error = %v, want ErrNotFound", err)
	}
}

func TestRenameNotFound(t *testing.T) {
	idx := openTest(t)
	if err := idx.Rename(context.Background(), 999, "new.txt"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Rename(999) error = %v, want ErrNotFound", err)
	}
}

func TestFindByHashMiss(t *testing.T) {
	idx := openTest(t)
	rec, ok, err := idx.FindByHash(context.Background(), "does-not-exist")
	if err != nil {
		t.Fatalf("FindByHash: unexpected error %v", err)
	}
	if ok || rec != nil {
		t.Fatalf("FindByHash miss: got ok=%v rec=%v, want ok=false rec=nil", ok, rec)
	}
}

func TestInsertDuplicateHash(t *testing.T) {
	idx := openTest(t)
	ctx := context.Background()
	rec := &Record{
		Path: "a.txt", Filename: "a.txt", Hash: "same-hash", Size: 1,
		MessageID: 1, ChannelID: 1, UploadedAt: time.Now().UTC(),
	}
	if err := idx.Insert(ctx, rec); err != nil {
		t.Fatalf("first insert: unexpected error %v", err)
	}

	dup := &Record{
		Path: "b.txt", Filename: "b.txt", Hash: "same-hash", Size: 2,
		MessageID: 2, ChannelID: 1, UploadedAt: time.Now().UTC(),
	}
	if err := idx.Insert(ctx, dup); err == nil {
		t.Fatal("second insert with duplicate hash: expected error, got nil")
	}
}

func TestRenameUpdatesFilenameOnly(t *testing.T) {
	idx := openTest(t)
	ctx := context.Background()
	rec := &Record{
		Path: "orig.txt", Filename: "orig.txt", Hash: "h1", Size: 1,
		MessageID: 1, ChannelID: 1, UploadedAt: time.Now().UTC(),
	}
	if err := idx.Insert(ctx, rec); err != nil {
		t.Fatalf("insert: %v", err)
	}

	if err := idx.Rename(ctx, rec.ID, "renamed.txt"); err != nil {
		t.Fatalf("rename: %v", err)
	}

	got, err := idx.Get(ctx, rec.ID)
	if err != nil {
		t.Fatalf("get after rename: %v", err)
	}
	if got.Filename != "renamed.txt" {
		t.Errorf("Filename = %q, want %q", got.Filename, "renamed.txt")
	}
	if got.Path != "orig.txt" {
		t.Errorf("Path = %q, want unchanged %q", got.Path, "orig.txt")
	}
}
