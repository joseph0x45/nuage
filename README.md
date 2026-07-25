# Nuage

Personal "free" cloud storage built on Telegram. A private Telegram channel
is the storage backend — Nuage builds the filesystem semantics (paths,
folders, dedup, per-user ownership) on top of it and gets native 2GB/4GB
(Premium) file size limits for free, without running any storage
infrastructure of your own.

Single static Go binary, SQLite for the local index, no containers, no
database server, no object storage bill.

## Features

- **Web UI** for day-to-day use: drag-and-drop upload, download, rename,
  delete, inline image/video preview, virtual folders.
- **Per-user login profiles** — each person logs in as their own profile and
  only sees/manages the files they uploaded. Dedup is scoped per-profile, so
  two people uploading the same file each keep their own copy (deleting one
  never affects the other's).
- **Content-hash dedup** — re-uploading the same file (by content, not name)
  is a no-op.
- **Bit-for-bit storage** — files are always sent as Telegram documents,
  never as a compressed photo/video, so nothing gets recompressed.
- **Disaster recovery without a second backup system** — every upload's
  metadata is mirrored into its Telegram message caption. If you ever lose
  the local SQLite index, `nuage reindex` rebuilds it from scratch by
  reading the channel's own message history.
- **Virtual folders** — purely a view over each file's stored path; no
  separate folder table, so organizing files costs nothing extra.

## How it works

```
┌───────────────┐
│  Web frontend │  <-- day-to-day interface
└──────┬────────┘
       │ HTTP/JSON
┌──────▼────────┐     ┌──────────────┐     ┌─────────────┐
│  Web server   │ --> │              │     │             │
└───────────────┘     │  Core engine │ --> │  gotd client│ --> Telegram
┌───────────────┐     │              │     │             │
│  CLI (setup)  │ --> │              │     └─────────────┘
└───────────────┘     └──────┬───────┘
                        ┌──────▼───────┐
                        │  SQLite index │
                        └───────────────┘
```

Nuage talks to Telegram as a real user account via MTProto
([`gotd/td`](https://github.com/gotd/td)), not the Bot API — that's what
gets the full 2GB/4GB file size limit instead of the Bot API's much lower
ceiling, without needing a companion local Bot API server. `message_id` +
`channel_id` is the durable reference to a file; uploads/downloads/renames/
deletes all go through the shared `internal/core` engine, used by both the
CLI and the web server so the logic only lives in one place.

See [`CLAUDE.md`](./CLAUDE.md) for the full architecture and the reasoning
behind each design decision.

## Requirements

- Go 1.26+ (only needed to build; the release binary has no runtime
  dependencies)
- A Telegram account
- API credentials from <https://my.telegram.org> (`api_id` / `api_hash`) —
  these identify the *application*, not you; free to create

## Build

```
make build     # builds ./nuage
make install   # builds and installs to ~/.local/bin/nuage
make test      # go test ./...
make vet       # go vet ./...
```

Or grab a prebuilt binary from the
[releases page](https://github.com/joseph0x45/nuage/releases) (Linux
amd64/arm64, Windows amd64).

## First-time setup

All of this is interactive (phone number, login code, 2FA, and later a web
login password), so it has to be run by hand in a real terminal — it's the
only thing the CLI is really for. Day-to-day file operations happen in the
web UI, not here.

```
nuage auth              # phone/code/2FA login; persists a Telegram session
nuage init               # creates (or picks, with --existing) the private
                          # channel Nuage stores files in
nuage user add <name>    # creates a web UI login profile; the first
                          # profile created also inherits any files already
                          # indexed before profiles existed
nuage serve               # starts the web server (binds 0.0.0.0:8080 by
                          # default; use --addr to change it)
```

Config, the Telegram session, and the SQLite index all live in
`$XDG_CONFIG_HOME/nuage` (falling back to `~/.config/nuage`). The session
file is a credential — treat it like one.

## CLI reference

The CLI is setup/scripting-only; it doesn't enforce per-profile ownership
the way the web UI does (files it touches are unscoped, `owner=""`).

| command | purpose |
|---|---|
| `nuage auth` | Telegram login (phone/code/2FA) |
| `nuage init [--title] [--about] [--existing]` | create or select the storage channel |
| `nuage user add <name>` | create a web login profile / change its password |
| `nuage user list` | list configured profiles |
| `nuage user rm <name>` | remove a profile's web login (files stay indexed under that name) |
| `nuage serve [--addr]` | start the web server |
| `nuage upload <path>` | upload a file from the CLI (scripting/debugging) |
| `nuage get <id> <dest-path>` | download an indexed file by id |
| `nuage list` | list every indexed file across all profiles |
| `nuage reindex` | rebuild `index.db` from scratch by scanning the channel's message history |
| `nuage backfill-captions` | one-time: write recovery metadata into the caption of files uploaded before that feature existed |

Run `nuage help` or `nuage <command> --help` for details on any of these.

## Deployment

`nuage serve` binds `0.0.0.0:8080` by default (override with `--addr`), so
it's exposable however you prefer — direct port-forward, a reverse proxy,
a [Cloudflare Tunnel](https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/),
whatever fits your setup. Per-profile login is the actual access control
regardless of how it's reached, not the network topology.

### As a systemd user service

A unit file is provided at [`deploy/nuage.service`](./deploy/nuage.service):

```
mkdir -p ~/.config/systemd/user
cp deploy/nuage.service ~/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user enable --now nuage.service
```

Two things matter for this to actually work:

1. **Enable linger** — without it, a `--user` service dies the moment you
   log out:
   ```
   sudo loginctl enable-linger <your-username>
   ```
2. **Finish first-time setup first** (`nuage auth` / `nuage init` /
   `nuage user add`, see above) — `nuage serve` refuses to start with no
   storage channel or no profiles configured, so the service will just
   crash-loop until that's done.

## Disaster recovery

If `index.db` is ever lost or corrupted, `nuage reindex` rebuilds it from
nothing but the Telegram channel itself — every upload's path, filename,
owner, and content hash are mirrored into that message's caption at upload
time. Files uploaded before this existed need a one-time
`nuage backfill-captions` run first to gain that metadata retroactively;
new uploads set it automatically.

## Out of scope

- Encryption — casual personal use, nothing sensitive stored
- A GUI beyond the web UI
- Auto-sync / watch-folder daemon
- The Bot API — MTProto was chosen specifically to avoid it (see
  `CLAUDE.md`)
