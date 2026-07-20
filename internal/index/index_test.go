package index

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
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
	rec, ok, err := idx.FindByHash(context.Background(), "does-not-exist", "joseph")
	if err != nil {
		t.Fatalf("FindByHash: unexpected error %v", err)
	}
	if ok || rec != nil {
		t.Fatalf("FindByHash miss: got ok=%v rec=%v, want ok=false rec=nil", ok, rec)
	}
}

func TestInsertDuplicateHashSameOwner(t *testing.T) {
	idx := openTest(t)
	ctx := context.Background()
	rec := &Record{
		Path: "a.txt", Filename: "a.txt", Owner: "joseph", Hash: "same-hash", Size: 1,
		MessageID: 1, ChannelID: 1, UploadedAt: time.Now().UTC(),
	}
	if err := idx.Insert(ctx, rec); err != nil {
		t.Fatalf("first insert: unexpected error %v", err)
	}

	dup := &Record{
		Path: "b.txt", Filename: "b.txt", Owner: "joseph", Hash: "same-hash", Size: 2,
		MessageID: 2, ChannelID: 1, UploadedAt: time.Now().UTC(),
	}
	if err := idx.Insert(ctx, dup); err == nil {
		t.Fatal("second insert with duplicate hash for the same owner: expected error, got nil")
	}
}

func TestInsertSameHashDifferentOwnersAllowed(t *testing.T) {
	idx := openTest(t)
	ctx := context.Background()
	a := &Record{
		Path: "a.txt", Filename: "a.txt", Owner: "joseph", Hash: "shared-hash", Size: 1,
		MessageID: 1, ChannelID: 1, UploadedAt: time.Now().UTC(),
	}
	if err := idx.Insert(ctx, a); err != nil {
		t.Fatalf("insert for joseph: unexpected error %v", err)
	}

	b := &Record{
		Path: "a.txt", Filename: "a.txt", Owner: "mom", Hash: "shared-hash", Size: 1,
		MessageID: 2, ChannelID: 1, UploadedAt: time.Now().UTC(),
	}
	if err := idx.Insert(ctx, b); err != nil {
		t.Fatalf("insert for mom with same hash as joseph's file: unexpected error %v", err)
	}
	if a.ID == b.ID {
		t.Fatal("expected distinct rows for distinct owners")
	}
}

func TestListScopesByOwner(t *testing.T) {
	idx := openTest(t)
	ctx := context.Background()
	for _, r := range []*Record{
		{Path: "a.txt", Filename: "a.txt", Owner: "joseph", Hash: "h1", Size: 1, MessageID: 1, ChannelID: 1, UploadedAt: time.Now().UTC()},
		{Path: "b.txt", Filename: "b.txt", Owner: "mom", Hash: "h2", Size: 1, MessageID: 2, ChannelID: 1, UploadedAt: time.Now().UTC()},
	} {
		if err := idx.Insert(ctx, r); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}

	josephFiles, err := idx.List(ctx, "joseph")
	if err != nil {
		t.Fatalf("list joseph: %v", err)
	}
	if len(josephFiles) != 1 || josephFiles[0].Owner != "joseph" {
		t.Fatalf("List(joseph) = %+v, want exactly joseph's file", josephFiles)
	}

	all, err := idx.List(ctx, "")
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("List(\"\") = %d records, want 2", len(all))
	}
}

func TestRenameUpdatesFilenameOnly(t *testing.T) {
	idx := openTest(t)
	ctx := context.Background()
	rec := &Record{
		Path: "orig.txt", Filename: "orig.txt", Owner: "joseph", Hash: "h1", Size: 1,
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

func TestMigrateOwnerColumnFromLegacySchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")

	// Recreate the pre-profiles schema directly (hash globally UNIQUE, no
	// owner column) and seed it with a row, bypassing Open so this test
	// exercises the migration path rather than the current schema.
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open raw db: %v", err)
	}
	if _, err := raw.Exec(`
		CREATE TABLE files (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			path        TEXT NOT NULL,
			filename    TEXT NOT NULL,
			hash        TEXT NOT NULL UNIQUE,
			size        INTEGER NOT NULL,
			message_id  INTEGER NOT NULL,
			channel_id  INTEGER NOT NULL,
			uploaded_at TIMESTAMP NOT NULL
		)`); err != nil {
		t.Fatalf("create legacy schema: %v", err)
	}
	if _, err := raw.Exec(
		`INSERT INTO files (path, filename, hash, size, message_id, channel_id, uploaded_at)
		 VALUES ('old.txt', 'old.txt', 'legacy-hash', 42, 7, 1, ?)`, time.Now().UTC()); err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("close raw db: %v", err)
	}

	idx, err := Open(path)
	if err != nil {
		t.Fatalf("Open on legacy db: %v", err)
	}
	defer idx.Close()

	records, err := idx.List(context.Background(), "")
	if err != nil {
		t.Fatalf("list after migration: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("got %d records after migration, want 1", len(records))
	}
	rec := records[0]
	if rec.Filename != "old.txt" || rec.Hash != "legacy-hash" || rec.Size != 42 || rec.MessageID != 7 {
		t.Errorf("migrated record = %+v, data was not preserved", rec)
	}
	if rec.Owner != "" {
		t.Errorf("Owner = %q, want empty (unbackfilled) after migration", rec.Owner)
	}

	// A second Open (simulating a server restart) must be a no-op, not fail
	// or re-run the migration against an already-migrated table.
	idx2, err := Open(path)
	if err != nil {
		t.Fatalf("second Open on migrated db: %v", err)
	}
	defer idx2.Close()
}

func TestBackfillOwner(t *testing.T) {
	idx := openTest(t)
	ctx := context.Background()
	legacy := &Record{
		Path: "legacy.txt", Filename: "legacy.txt", Owner: "", Hash: "h1", Size: 1,
		MessageID: 1, ChannelID: 1, UploadedAt: time.Now().UTC(),
	}
	if err := idx.Insert(ctx, legacy); err != nil {
		t.Fatalf("insert: %v", err)
	}

	if err := idx.BackfillOwner(ctx, "joseph"); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	got, err := idx.Get(ctx, legacy.ID)
	if err != nil {
		t.Fatalf("get after backfill: %v", err)
	}
	if got.Owner != "joseph" {
		t.Errorf("Owner = %q, want %q", got.Owner, "joseph")
	}
}
