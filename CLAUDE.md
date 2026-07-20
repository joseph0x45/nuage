# Nuage

Personal "free" cloud storage built on Telegram, using a private channel as the
storage backend. Primary user: a non-technical family member with thousands of
saved images (Pinterest-style) and occasional video files. No encryption needed —
nothing sensitive is stored here.

## Goals

- Use Telegram as dumb blob storage; build the actual filesystem semantics ourselves.
- Support files up to Telegram's native limits (2GB free / 4GB Premium) without
  building our own sub-file chunking — MTProto already handles that internally.
- Chunking is only needed as a rare edge case, for files above the hard per-account
  ceiling.
- Bit-for-bit file preservation (no recompression) — always send as document/file,
  never as compressed photo/video, since Telegram recompresses `sendPhoto`/`sendVideo`.
- Dedup via content hash before upload.
- Web interface is the primary way this will be used day-to-day. CLI exists mainly
  for first-time setup (Telegram auth, channel config) — not the main interaction
  surface.
- Build the core engine (upload/download/index/dedup) as a reusable package first,
  since both CLI and web server will call into it — avoid duplicating logic between
  the two entry points.

## Stack

- **Language**: Go
- **Telegram layer**: MTProto via [`gotd/td`](https://github.com/gotd/td) — logging
  in as a real user account, not the Bot API. This was chosen specifically to get
  native 2-4GB file size limits without running a companion local Bot API server.
- **Local index**: SQLite
- **CLI framework**: cobra (tentative — open to alternatives), used for setup only
- **Web backend**: Go HTTP server (net/http or a light router like chi) exposing
  a JSON API over the core engine package
- **Web frontend**: not yet decided — simple server-rendered templates vs a small
  JS frontend (e.g. htmx or a minimal React app) is an open question. Given the
  main need is "browse/upload/download files," leaning toward keeping this simple
  rather than a full SPA.

## Architecture

```
┌───────────────┐
│  Web frontend │  <-- primary day-to-day interface
└──────┬────────┘
       │ HTTP/JSON
┌──────▼────────┐     ┌──────────────┐     ┌─────────────┐
│  Web server   │ --> │              │     │             │
└───────────────┘     │  Core engine │ --> │  gotd client│ --> Telegram
┌───────────────┐     │              │     │             │
│  CLI (cobra)  │ --> │              │     └─────────────┘
└───────────────┘     └──────┬───────┘
  (setup only)                │
                        ┌──────▼───────┐
                        │  SQLite index │
                        └───────────────┘
```

Both the CLI and the web server are thin entry points into the same core engine
package — neither should contain upload/download/index logic directly.

### Auth

- Requires `api_id` / `api_hash` from my.telegram.org (identifies the *application*,
  config file, not hardcoded).
- First run does interactive phone/code/2FA login via `gotd/td`'s `auth.Flow`
  (implement `auth.UserAuthenticator` for prompting).
- Session persisted via `session.FileStorage` to a local file after first login —
  treat this file as a credential (gitignore, restrictive permissions). Without it,
  every run would require re-authentication.

### Storage target

- A private Telegram channel (not Saved Messages/DM) that the account is a member/admin
  of. Gives a clean, independently scrollable archive.
- Store both `channel_id` and `access_hash` in config — gotd needs both to address
  the channel (`InputPeerChannel`).

### Upload/download flow

```
file → hash → dedup check against index
     → uploader.Upload() (gotd's uploader package; internally handles
       MTProto big-file part splitting, configurable concurrency via
       uploader.WithThreads(n))
     → messages.SendMedia(InputPeerChannel, InputFile) → returns message_id
     → write {path, message_id, hash, size, upload_date} row to SQLite
```

Download is the mirror: look up `message_id` in index → `messages.GetMessages` →
resolve `InputFileLocation` from the media → `downloader.Download()` to disk.

**message_id + channel_id is the durable reference for a file.** Telegram's
`file_id` can rotate/expire over long periods — don't rely on it as the primary key.

### Index schema (SQLite) — draft

| column       | notes                                   |
|--------------|------------------------------------------|
| id           | primary key                              |
| path         | virtual path / original relative path    |
| filename     | original filename                        |
| owner        | username of the profile that uploaded it (empty = pre-profiles legacy file) |
| hash         | content hash, `UNIQUE(owner, hash)` — dedup is scoped per-owner, not global |
| size         | bytes                                    |
| message_id   | Telegram message id (durable pointer)    |
| channel_id   | Telegram channel id                      |
| uploaded_at  | timestamp                                |

Open question: whether to also mirror this index into Telegram itself (e.g. a
pinned message or dedicated index channel) so it can be rebuilt from scratch on
a new machine. Not yet decided.

### Chunking (edge case only)

Only needed for files above the hard per-account ceiling (4GB w/ Premium, 2GB
without). Design when needed:
- Fixed-size chunks (~1.9GB) written to temp, each chunk = its own Telegram message.
- Manifest per file: ordered chunk message_ids + per-chunk hash + whole-file hash
  for reassembly verification.
- Reassembly on download: fetch chunks in order, concatenate, verify hash.

## CLI commands (setup-only, v1 target)

- `nuage auth` — run first-time Telegram login flow (phone/code/2FA), write session file
- `nuage init` — create/configure the storage channel, write config
- `nuage serve` — start the web server (this becomes the normal way to run Nuage
  day-to-day, even though it's invoked from a CLI)

Upload/download/list/verify as user-facing actions live in the web interface, not
as separate CLI subcommands, since the CLI isn't the main interaction surface.
(A thin `nuage upload`/`nuage get` may still be worth keeping for scripting/debugging,
but isn't the target UX.)

## Web interface

- Backend: Go HTTP server exposing a JSON API over the core engine (upload, list,
  download, dedup-check, delete).
- Frontend: browse files/folders, drag-and-drop or file-picker upload, download,
  progress indication for large video uploads. Needs to be simple enough for a
  non-technical user (primary user is not you).
- **Auth for the web UI is required, not optional** — since this will be reachable
  over the internet via Cloudflare Tunnel (not just LAN). Each household member logs
  in as a named profile (`nuage user add <username>`, bcrypt-hashed password) rather
  than a single shared password — each profile only sees/manages the files it
  uploaded. Worth considering Cloudflare Access (zero-trust policy in front of the
  tunnel, e.g. email OTP) as a second layer, since it stops unauthenticated requests
  before they even reach the Go server.
- Deployment target: home server, exposed to the internet via a Cloudflare Tunnel
  (no direct port-forwarding). `nuage serve` should bind to localhost/LAN and let
  `cloudflared` handle the public-facing side — the Go server itself never needs
  to listen on a public interface directly.
- Rate limiting / brute-force protection on the login endpoint is worth having
  once this is internet-reachable, keyed per-IP regardless of which profile is
  being logged into.

## Explicitly out of scope for v1

- Encryption (not needed — no sensitive data)
- Watch-folder daemon / auto-sync — not needed
- GUI
- Bot API / local Bot API server path (rejected in favor of MTProto — see below)

## Decisions already made (don't re-litigate without reason)

- **MTProto over Bot API**: chosen for native 2-4GB limits without running a
  local Bot API server as a companion process.
- **gotd/td over other options**: most mature actively-maintained Go MTProto
  implementation.
- **Private channel over Saved Messages**: cleaner as a dedicated archive.
- **No encryption**: explicit user decision, casual personal use case.
- **Document upload, not photo/video upload**: avoids Telegram's recompression.
- **Web interface is primary, CLI is setup-only**: day-to-day use (upload, browse,
  download) happens through the web UI, not CLI subcommands.
- **Deployment: home server, exposed via Cloudflare Tunnel**: internet-accessible,
  not LAN-only — auth on the web UI is mandatory, not optional. `nuage serve` binds
  locally; `cloudflared` handles public exposure, so the Go server never listens
  on a public interface directly.
- **Named per-user profiles, not a single shared password**: each household member
  (you, your mom) logs in as their own profile and only sees/manages their own
  files. Dedup is scoped per-owner rather than global (`UNIQUE(owner, hash)`) —
  two profiles uploading the same content each get their own Telegram message,
  traded deliberately against a shared/ref-counted-delete design to avoid one
  profile's delete ever being able to break another's file.
