// Package index is Nuage's local SQLite record of what's been uploaded:
// virtual path, content hash (for dedup), and the Telegram message_id/
// channel_id pair needed to fetch the file back. message_id+channel_id is
// the durable reference — Telegram's file_id can rotate over time.
package index

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// Record is one uploaded file's index entry.
type Record struct {
	ID         int64
	Path       string
	Filename   string
	Hash       string
	Size       int64
	MessageID  int
	ChannelID  int64
	UploadedAt time.Time
}

// Index wraps the SQLite-backed file index.
type Index struct {
	db *sql.DB
}

// ErrNotFound is returned (wrapped) by Get when id has no matching record.
var ErrNotFound = sql.ErrNoRows

const schema = `
CREATE TABLE IF NOT EXISTS files (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	path        TEXT NOT NULL,
	filename    TEXT NOT NULL,
	hash        TEXT NOT NULL UNIQUE,
	size        INTEGER NOT NULL,
	message_id  INTEGER NOT NULL,
	channel_id  INTEGER NOT NULL,
	uploaded_at TIMESTAMP NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_files_path ON files(path);
`

// Open opens (creating if needed) the SQLite database at path and ensures
// the schema exists.
func Open(path string) (*Index, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open index db at %s: %w", path, err)
	}
	// SQLite only supports one writer at a time; a single connection avoids
	// SQLITE_BUSY errors under concurrent access from the web server.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	return &Index{db: db}, nil
}

func (idx *Index) Close() error {
	return idx.db.Close()
}

// Insert records a newly uploaded file. rec.ID is ignored and populated on
// return.
func (idx *Index) Insert(ctx context.Context, rec *Record) error {
	res, err := idx.db.ExecContext(ctx,
		`INSERT INTO files (path, filename, hash, size, message_id, channel_id, uploaded_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		rec.Path, rec.Filename, rec.Hash, rec.Size, rec.MessageID, rec.ChannelID, rec.UploadedAt,
	)
	if err != nil {
		return fmt.Errorf("insert file record: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("read inserted id: %w", err)
	}
	rec.ID = id
	return nil
}

// FindByHash looks up an existing record by content hash, for dedup checks
// before uploading. ok is false if no record matches.
func (idx *Index) FindByHash(ctx context.Context, hash string) (*Record, bool, error) {
	rec, err := scanOne(idx.db.QueryRowContext(ctx,
		`SELECT id, path, filename, hash, size, message_id, channel_id, uploaded_at
		 FROM files WHERE hash = ?`, hash))
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("find by hash: %w", err)
	}
	return rec, true, nil
}

// Get looks up a record by its index id.
func (idx *Index) Get(ctx context.Context, id int64) (*Record, error) {
	rec, err := scanOne(idx.db.QueryRowContext(ctx,
		`SELECT id, path, filename, hash, size, message_id, channel_id, uploaded_at
		 FROM files WHERE id = ?`, id))
	if err != nil {
		return nil, fmt.Errorf("get record %d: %w", id, err)
	}
	return rec, nil
}

// Delete removes the record for id. It does not touch Telegram — callers
// are responsible for deleting the underlying message first.
func (idx *Index) Delete(ctx context.Context, id int64) error {
	res, err := idx.db.ExecContext(ctx, `DELETE FROM files WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete record %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("check delete result for %d: %w", id, err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// Rename updates the filename for id. It leaves Path untouched — Path and
// Filename only track together at upload time; once folder support exists
// Path may diverge.
func (idx *Index) Rename(ctx context.Context, id int64, filename string) error {
	res, err := idx.db.ExecContext(ctx, `UPDATE files SET filename = ? WHERE id = ?`, filename, id)
	if err != nil {
		return fmt.Errorf("rename record %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rename result for %d: %w", id, err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// List returns every indexed file, ordered by upload time (most recent
// first).
func (idx *Index) List(ctx context.Context) ([]*Record, error) {
	rows, err := idx.db.QueryContext(ctx,
		`SELECT id, path, filename, hash, size, message_id, channel_id, uploaded_at
		 FROM files ORDER BY uploaded_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list files: %w", err)
	}
	defer rows.Close()

	var records []*Record
	for rows.Next() {
		var rec Record
		if err := rows.Scan(&rec.ID, &rec.Path, &rec.Filename, &rec.Hash, &rec.Size,
			&rec.MessageID, &rec.ChannelID, &rec.UploadedAt); err != nil {
			return nil, fmt.Errorf("scan file row: %w", err)
		}
		records = append(records, &rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate file rows: %w", err)
	}
	return records, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanOne(row rowScanner) (*Record, error) {
	var rec Record
	if err := row.Scan(&rec.ID, &rec.Path, &rec.Filename, &rec.Hash, &rec.Size,
		&rec.MessageID, &rec.ChannelID, &rec.UploadedAt); err != nil {
		return nil, err
	}
	return &rec, nil
}
