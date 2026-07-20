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
	"os"
	"time"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/message"
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
// and records it under filename in the index, owned by owner. filename is
// separate from srcPath because callers (e.g. the web server) may stage the
// upload under a temp path distinct from the name the user gave it. If
// owner already uploaded this exact content, the existing record is
// returned as-is (no re-upload) — dedup is scoped per-owner, so a different
// owner uploading the same content still gets their own copy (see the
// schema comment in internal/index). owner is "" for the unscoped CLI
// commands.
func (e *Engine) Upload(ctx context.Context, srcPath, filename, owner string) (*index.Record, error) {
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

	updates, err := e.sender.To(e.channel).File(ctx, inputFile)
	if err != nil {
		return nil, fmt.Errorf("send %s as document: %w", srcPath, err)
	}

	msgID, err := messageID(updates)
	if err != nil {
		return nil, err
	}

	rec := &index.Record{
		Path:       filename,
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

// Rename updates the display name of the file recorded under id. It only
// touches the index — the underlying Telegram message is untouched. See
// Download for owner's semantics.
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
	return e.idx.Get(ctx, id)
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
