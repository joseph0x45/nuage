// Package core is Nuage's shared engine: upload, download, list, and dedup
// logic used by both the setup CLI and the web server. Neither entry point
// should talk to gotd or the index directly — only through this package.
package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"strings"
	"time"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/telegram/message/styling"
	"github.com/gotd/td/telegram/query"
	"github.com/gotd/td/telegram/query/messages"
	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"

	"github.com/joseph0x45/nuage/internal/config"
	"github.com/joseph0x45/nuage/internal/index"
	nuagetg "github.com/joseph0x45/nuage/internal/telegram"
)

// UploadThreads is the concurrency used for splitting large files into
// MTProto parts during upload.
const UploadThreads = 4

// Engine ties together a live, authenticated Telegram connection, the
// storage channel, and the local SQLite index. It owns the connection's
// lifecycle: construct with New, and call Close when done.
type Engine struct {
	client   *telegram.Client
	api      *tg.Client
	idx      *index.Index
	channel  *tg.InputPeerChannel
	uploader *uploader.Uploader
	sender   *message.Sender

	cancel context.CancelFunc
	done   chan error
}

// New connects to Telegram using cfg's credentials and persisted session,
// and opens the local index. The returned Engine is ready to use
// immediately; call Close to release both.
func New(cfg *config.Config, idx *index.Index) (*Engine, error) {
	if cfg.ChannelID == 0 {
		return nil, fmt.Errorf("no storage channel configured; run `nuage init` first")
	}

	client, err := nuagetg.NewClient(cfg)
	if err != nil {
		return nil, err
	}

	runCtx, cancel := context.WithCancel(context.Background())
	ready := make(chan error, 1)
	done := make(chan error, 1)

	go func() {
		done <- client.Run(runCtx, func(ctx context.Context) error {
			status, err := client.Auth().Status(ctx)
			if err != nil {
				ready <- fmt.Errorf("check auth status: %w", err)
				return nil
			}
			if !status.Authorized {
				ready <- fmt.Errorf("not logged in; run `nuage auth` first")
				return nil
			}
			ready <- nil
			<-ctx.Done()
			return nil
		})
	}()

	if err := <-ready; err != nil {
		cancel()
		<-done
		return nil, err
	}

	api := client.API()
	up := uploader.NewUploader(api).WithThreads(UploadThreads)

	return &Engine{
		client: client,
		api:    api,
		idx:    idx,
		channel: &tg.InputPeerChannel{
			ChannelID:  cfg.ChannelID,
			AccessHash: cfg.ChannelAccessHash,
		},
		uploader: up,
		sender:   message.NewSender(api).WithUploader(up),
		cancel:   cancel,
		done:     done,
	}, nil
}

// Close tears down the Telegram connection. It does not close the index —
// callers own that separately since it may outlive a single Engine.
func (e *Engine) Close() error {
	e.cancel()
	return <-e.done
}

// Upload hashes the file at srcPath, checks the index for an existing
// upload by owner with the same content, and if none exists, sends it to
// the storage channel as a plain document (never a compressed photo/video)
// and records it under virtualPath in the index, owned by owner. srcPath is
// separate from virtualPath because callers (e.g. the web server) may stage
// the upload under a temp path distinct from the name the user gave it.
// virtualPath may include a folder prefix (e.g. "vacation/2024/photo.png")
// — folders are purely virtual, derived from Path prefixes at listing time,
// not stored anywhere of their own. If owner already uploaded this exact
// content, the existing record is returned as-is (no re-upload) — dedup is
// scoped per-owner, so a different owner uploading the same content still
// gets their own copy (see the schema comment in internal/index). owner is
// "" for the unscoped CLI commands.
func (e *Engine) Upload(ctx context.Context, srcPath, virtualPath, owner string) (*index.Record, error) {
	virtualPath = normalizeVirtualPath(virtualPath)
	if virtualPath == "" {
		return nil, fmt.Errorf("empty upload path")
	}
	filename := path.Base(virtualPath)

	hash, size, err := hashFile(srcPath)
	if err != nil {
		return nil, err
	}

	if existing, ok, err := e.idx.FindByHash(ctx, hash, owner); err != nil {
		return nil, err
	} else if ok {
		return existing, nil
	}

	inputFile, err := e.uploader.FromPath(ctx, srcPath)
	if err != nil {
		return nil, fmt.Errorf("upload %s to telegram: %w", srcPath, err)
	}

	// The caption mirrors just enough metadata (path/filename/owner/hash)
	// for Reindex to rebuild the local index from the channel alone if
	// index.db is ever lost — everything else (size, message id, upload
	// date) is recovered from the message itself.
	caption, err := encodeFileMeta(&index.Record{Path: virtualPath, Filename: filename, Owner: owner, Hash: hash})
	if err != nil {
		return nil, fmt.Errorf("encode file metadata: %w", err)
	}

	updates, err := e.sender.To(e.channel).File(ctx, inputFile, styling.Plain(caption))
	if err != nil {
		return nil, fmt.Errorf("send %s as document: %w", srcPath, err)
	}

	msgID, err := messageID(updates)
	if err != nil {
		return nil, err
	}

	rec := &index.Record{
		Path:       virtualPath,
		Filename:   filename,
		Owner:      owner,
		Hash:       hash,
		Size:       size,
		MessageID:  msgID,
		ChannelID:  e.channel.ChannelID,
		UploadedAt: time.Now().UTC(),
	}
	if err := e.idx.Insert(ctx, rec); err != nil {
		return nil, err
	}
	return rec, nil
}

// normalizeVirtualPath cleans a caller-supplied virtual path into a
// consistent forward-slash form with no leading slash, resolving any ".."
// or "." segments. Anchoring at "/" before cleaning means a stray ".." can
// never escape above the root — it's just discarded, same as path.Clean
// does for an OS root. Returns "" for a path that cleans down to nothing
// (root itself, or all-".." input).
func normalizeVirtualPath(p string) string {
	p = strings.TrimPrefix(path.Clean("/"+p), "/")
	if p == "." {
		return ""
	}
	return p
}

// Download fetches the file recorded under id in the index and writes it to
// destPath. owner must match the record's owner, or be "" to bypass the
// check (the unscoped CLI commands); otherwise index.ErrNotFound is
// returned, matching the id-not-found case so callers can't distinguish
// "doesn't exist" from "belongs to someone else".
func (e *Engine) Download(ctx context.Context, id int64, destPath, owner string) (*index.Record, error) {
	rec, loc, err := e.locateDocument(ctx, id, owner)
	if err != nil {
		return nil, err
	}
	if _, err := e.client.Download(loc).ToPath(ctx, destPath); err != nil {
		return nil, fmt.Errorf("download to %s: %w", destPath, err)
	}
	return rec, nil
}

// Stream fetches the file recorded under id and writes its bytes directly
// to w, for the web server to pipe into an HTTP response without staging a
// temp file on disk. See Download for owner's semantics.
func (e *Engine) Stream(ctx context.Context, id int64, w io.Writer, owner string) (*index.Record, error) {
	rec, loc, err := e.locateDocument(ctx, id, owner)
	if err != nil {
		return nil, err
	}
	if _, err := e.client.Download(loc).Stream(ctx, w); err != nil {
		return nil, fmt.Errorf("stream download: %w", err)
	}
	return rec, nil
}

func (e *Engine) locateDocument(ctx context.Context, id int64, owner string) (*index.Record, tg.InputFileLocationClass, error) {
	rec, err := e.idx.Get(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	if owner != "" && rec.Owner != owner {
		return nil, nil, index.ErrNotFound
	}

	res, err := e.api.ChannelsGetMessages(ctx, &tg.ChannelsGetMessagesRequest{
		Channel: &tg.InputChannel{
			ChannelID:  e.channel.ChannelID,
			AccessHash: e.channel.AccessHash,
		},
		ID: []tg.InputMessageClass{&tg.InputMessageID{ID: rec.MessageID}},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("fetch message %d: %w", rec.MessageID, err)
	}

	channelMsgs, ok := res.(*tg.MessagesChannelMessages)
	if !ok || len(channelMsgs.Messages) == 0 {
		return nil, nil, fmt.Errorf("message %d not found in storage channel", rec.MessageID)
	}
	msg, ok := channelMsgs.Messages[0].(*tg.Message)
	if !ok {
		return nil, nil, fmt.Errorf("message %d is not a regular message", rec.MessageID)
	}
	media, ok := msg.Media.(*tg.MessageMediaDocument)
	if !ok {
		return nil, nil, fmt.Errorf("message %d has no document attached", rec.MessageID)
	}
	doc, ok := media.Document.(*tg.Document)
	if !ok {
		return nil, nil, fmt.Errorf("message %d document is unavailable", rec.MessageID)
	}

	return rec, doc.AsInputDocumentFileLocation(""), nil
}

// List returns indexed files. An empty owner returns every file regardless
// of who uploaded it (the unscoped CLI commands); the web server always
// passes the logged-in profile's username.
func (e *Engine) List(ctx context.Context, owner string) ([]*index.Record, error) {
	return e.idx.List(ctx, owner)
}

// Record looks up a single indexed file's metadata by id, without touching
// Telegram. Callers that need the file bytes should follow up with Stream
// or Download. See Download for owner's semantics.
func (e *Engine) Record(ctx context.Context, id int64, owner string) (*index.Record, error) {
	rec, err := e.idx.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if owner != "" && rec.Owner != owner {
		return nil, index.ErrNotFound
	}
	return rec, nil
}

// Rename updates the display name of the file recorded under id, and
// best-effort updates the Telegram message's caption to match so Reindex
// stays accurate. The index update is authoritative — a caption-edit
// failure is logged, not returned, since day-to-day serving only ever
// reads from the index. See Download for owner's semantics.
func (e *Engine) Rename(ctx context.Context, id int64, filename, owner string) (*index.Record, error) {
	rec, err := e.idx.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if owner != "" && rec.Owner != owner {
		return nil, index.ErrNotFound
	}
	if err := e.idx.Rename(ctx, id, filename); err != nil {
		return nil, err
	}
	rec, err = e.idx.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	if caption, err := encodeFileMeta(rec); err != nil {
		log.Printf("encode file metadata for renamed file %d: %v", id, err)
	} else if _, err := e.sender.To(e.channel).Edit(rec.MessageID).Text(ctx, caption); err != nil {
		log.Printf("update telegram caption for renamed file %d (message %d): %v", id, rec.MessageID, err)
	}

	return rec, nil
}

// Delete removes the file recorded under id: first the message in the
// storage channel, then the index row. If the message was already gone
// from Telegram (e.g. deleted manually), the index row is still removed.
// See Download for owner's semantics.
func (e *Engine) Delete(ctx context.Context, id int64, owner string) error {
	rec, err := e.idx.Get(ctx, id)
	if err != nil {
		return err
	}
	if owner != "" && rec.Owner != owner {
		return index.ErrNotFound
	}

	_, err = e.api.ChannelsDeleteMessages(ctx, &tg.ChannelsDeleteMessagesRequest{
		Channel: &tg.InputChannel{
			ChannelID:  e.channel.ChannelID,
			AccessHash: e.channel.AccessHash,
		},
		ID: []int{rec.MessageID},
	})
	if err != nil {
		return fmt.Errorf("delete message %d: %w", rec.MessageID, err)
	}

	return e.idx.Delete(ctx, id)
}

// Reindex wipes the local index and rebuilds it from scratch by walking
// every message in the storage channel and parsing the metadata each
// upload's caption carries (see fileMeta). Use this after losing index.db,
// or when moving to a new machine. Messages without recognizable Nuage
// metadata (e.g. anything posted before this feature existed, or manually)
// are silently skipped — run BackfillCaptions first to cover older uploads.
// Returns how many records were recovered.
func (e *Engine) Reindex(ctx context.Context) (int, error) {
	if err := e.idx.Reset(ctx); err != nil {
		return 0, err
	}

	var count int
	err := query.Messages(e.api).GetHistory(e.channel).BatchSize(100).
		ForEach(ctx, func(ctx context.Context, elem messages.Elem) error {
			msg, ok := elem.Msg.(*tg.Message)
			if !ok {
				return nil
			}
			media, ok := msg.Media.(*tg.MessageMediaDocument)
			if !ok {
				return nil
			}
			doc, ok := media.Document.(*tg.Document)
			if !ok {
				return nil
			}
			meta, ok := decodeFileMeta(msg.Message)
			if !ok {
				return nil
			}

			rec := &index.Record{
				Path:       meta.Path,
				Filename:   meta.Filename,
				Owner:      meta.Owner,
				Hash:       meta.Hash,
				Size:       doc.Size,
				MessageID:  msg.ID,
				ChannelID:  e.channel.ChannelID,
				UploadedAt: time.Unix(int64(msg.Date), 0).UTC(),
			}
			if err := e.idx.Insert(ctx, rec); err != nil {
				return fmt.Errorf("insert reindexed record for message %d: %w", msg.ID, err)
			}
			count++
			return nil
		})
	if err != nil {
		return count, fmt.Errorf("walk channel history: %w", err)
	}
	return count, nil
}

// BackfillCaptions writes disaster-recovery metadata into the Telegram
// caption of every currently-indexed file, for files uploaded before
// captions were introduced (new uploads set theirs immediately in Upload).
// Only needs to run once; safe to re-run since Edit overwrites in place.
// Returns how many messages were updated before any error.
func (e *Engine) BackfillCaptions(ctx context.Context) (int, error) {
	records, err := e.idx.List(ctx, "")
	if err != nil {
		return 0, err
	}

	var count int
	for _, rec := range records {
		caption, err := encodeFileMeta(rec)
		if err != nil {
			return count, fmt.Errorf("encode file metadata for file %d: %w", rec.ID, err)
		}
		if _, err := e.sender.To(e.channel).Edit(rec.MessageID).Text(ctx, caption); err != nil {
			return count, fmt.Errorf("set caption for file %d (message %d): %w", rec.ID, rec.MessageID, err)
		}
		count++
	}
	return count, nil
}

// messageID extracts the id of the message just sent from the UpdatesClass
// returned by a send call.
func messageID(updates tg.UpdatesClass) (int, error) {
	u, ok := updates.(*tg.Updates)
	if !ok {
		return 0, fmt.Errorf("unexpected response type %T from send", updates)
	}
	for _, upd := range u.Updates {
		if nm, ok := upd.(*tg.UpdateNewChannelMessage); ok {
			return nm.Message.GetID(), nil
		}
	}
	return 0, fmt.Errorf("send response did not include a new message update")
}

func hashFile(path string) (hash string, size int64, err error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, fmt.Errorf("hash %s: %w", path, err)
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}
