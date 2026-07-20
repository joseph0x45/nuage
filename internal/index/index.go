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

// Record is one uploaded file's index entry. Owner is the username of the
// profile that uploaded it (empty for files uploaded before profiles
// existed, or via the unscoped CLI commands).
type Record struct {
	ID         int64
	Path       string
	Filename   string
	Owner      string
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

// hash is UNIQUE per-owner rather than globally: dedup is scoped to each
// profile, so two different users uploading the same content each get their
// own row (and their own copy on Telegram) — avoiding a shared-row design
// where one user's delete could break another user's file.
const schema = `
CREATE TABLE IF NOT EXISTS files (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	path        TEXT NOT NULL,
	filename    TEXT NOT NULL,
	owner       TEXT NOT NULL DEFAULT '',
	hash        TEXT NOT NULL,
	size        INTEGER NOT NULL,
	message_id  INTEGER NOT NULL,
	channel_id  INTEGER NOT NULL,
	uploaded_at TIMESTAMP NOT NULL,
	UNIQUE(owner, hash)
);
CREATE INDEX IF NOT EXISTS idx_files_path ON files(path);
`

// Open opens (creating if needed) the SQLite database at path, ensures the
// schema exists, and migrates it forward if it predates the owner column.
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
	if err := migrateOwnerColumn(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate schema: %w", err)
	}
	// The owner column is guaranteed to exist by this point (either the
	// CREATE TABLE above already had it, or migrateOwnerColumn just added
	// it), so this index can only safely be created here — doing it inside
	// the schema constant above would break on pre-migration databases.
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_files_owner ON files(owner)`); err != nil {
		db.Close()
		return nil, fmt.Errorf("create owner index: %w", err)
	}
	return &Index{db: db}, nil
}

// migrateOwnerColumn upgrades a pre-profiles database (hash globally UNIQUE,
// no owner column) to the current schema. Existing rows get owner = ” —
// BackfillOwner assigns them to a real profile once one exists.
//
// The old UNIQUE(hash) was declared inline on the column, so it can't be
// dropped in place; the table has to be rebuilt. AUTOINCREMENT ids survive
// this because SQLite bumps sqlite_sequence to the highest id it has ever
// seen, including explicitly-inserted ones from the copy below.
func migrateOwnerColumn(db *sql.DB) error {
	hasOwner, err := hasColumn(db, "files", "owner")
	if err != nil {
		return err
	}
	if hasOwner {
		return nil
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`ALTER TABLE files RENAME TO files_old`); err != nil {
		return fmt.Errorf("rename old table: %w", err)
	}
	if _, err := tx.Exec(`
		CREATE TABLE files (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			path        TEXT NOT NULL,
			filename    TEXT NOT NULL,
			owner       TEXT NOT NULL DEFAULT '',
			hash        TEXT NOT NULL,
			size        INTEGER NOT NULL,
			message_id  INTEGER NOT NULL,
			channel_id  INTEGER NOT NULL,
			uploaded_at TIMESTAMP NOT NULL,
			UNIQUE(owner, hash)
		)`); err != nil {
		return fmt.Errorf("create new table: %w", err)
	}
	if _, err := tx.Exec(`
		INSERT INTO files (id, path, filename, owner, hash, size, message_id, channel_id, uploaded_at)
		SELECT id, path, filename, '', hash, size, message_id, channel_id, uploaded_at FROM files_old`); err != nil {
		return fmt.Errorf("copy rows: %w", err)
	}
	if _, err := tx.Exec(`DROP TABLE files_old`); err != nil {
		return fmt.Errorf("drop old table: %w", err)
	}
	if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_files_path ON files(path)`); err != nil {
		return fmt.Errorf("recreate path index: %w", err)
	}
	if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_files_owner ON files(owner)`); err != nil {
		return fmt.Errorf("create owner index: %w", err)
	}

	return tx.Commit()
}

func hasColumn(db *sql.DB, table, column string) (bool, error) {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return false, fmt.Errorf("inspect %s schema: %w", table, err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return false, fmt.Errorf("scan column info: %w", err)
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}

// BackfillOwner assigns every unowned row (owner = ”) to owner. Intended
// to run once, when the first user profile is created, so files uploaded
// before profiles existed become that profile's files.
func (idx *Index) BackfillOwner(ctx context.Context, owner string) error {
	_, err := idx.db.ExecContext(ctx, `UPDATE files SET owner = ? WHERE owner = ''`, owner)
	if err != nil {
		return fmt.Errorf("backfill owner: %w", err)
	}
	return nil
}

func (idx *Index) Close() error {
	return idx.db.Close()
}

// Insert records a newly uploaded file. rec.ID is ignored and populated on
// return.
func (idx *Index) Insert(ctx context.Context, rec *Record) error {
	res, err := idx.db.ExecContext(ctx,
		`INSERT INTO files (path, filename, owner, hash, size, message_id, channel_id, uploaded_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		rec.Path, rec.Filename, rec.Owner, rec.Hash, rec.Size, rec.MessageID, rec.ChannelID, rec.UploadedAt,
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

// FindByHash looks up an existing record by content hash within owner's
// files, for dedup checks before uploading. Dedup is scoped per-owner —
// see the schema comment on the UNIQUE(owner, hash) constraint. ok is false
// if no record matches.
func (idx *Index) FindByHash(ctx context.Context, hash, owner string) (*Record, bool, error) {
	rec, err := scanOne(idx.db.QueryRowContext(ctx,
		`SELECT id, path, filename, owner, hash, size, message_id, channel_id, uploaded_at
		 FROM files WHERE hash = ? AND owner = ?`, hash, owner))
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("find by hash: %w", err)
	}
	return rec, true, nil
}

// Get looks up a record by its index id, regardless of owner — callers that
// need to enforce ownership should check rec.Owner themselves.
func (idx *Index) Get(ctx context.Context, id int64) (*Record, error) {
	rec, err := scanOne(idx.db.QueryRowContext(ctx,
		`SELECT id, path, filename, owner, hash, size, message_id, channel_id, uploaded_at
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

// Reset deletes every row, for Reindex to rebuild from scratch by scanning
// the storage channel — see Engine.Reindex.
func (idx *Index) Reset(ctx context.Context) error {
	if _, err := idx.db.ExecContext(ctx, `DELETE FROM files`); err != nil {
		return fmt.Errorf("reset index: %w", err)
	}
	return nil
}

// List returns indexed files ordered by upload time (most recent first).
// An empty owner returns every file regardless of who uploaded it — used by
// the unscoped CLI commands; the web server always passes the logged-in
// profile's username.
func (idx *Index) List(ctx context.Context, owner string) ([]*Record, error) {
	query := `SELECT id, path, filename, owner, hash, size, message_id, channel_id, uploaded_at
		 FROM files`
	args := []any{}
	if owner != "" {
		query += ` WHERE owner = ?`
		args = append(args, owner)
	}
	query += ` ORDER BY uploaded_at DESC`

	rows, err := idx.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list files: %w", err)
	}
	defer rows.Close()

	var records []*Record
	for rows.Next() {
		var rec Record
		if err := rows.Scan(&rec.ID, &rec.Path, &rec.Filename, &rec.Owner, &rec.Hash, &rec.Size,
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
	if err := row.Scan(&rec.ID, &rec.Path, &rec.Filename, &rec.Owner, &rec.Hash, &rec.Size,
		&rec.MessageID, &rec.ChannelID, &rec.UploadedAt); err != nil {
		return nil, err
	}
	return &rec, nil
}
