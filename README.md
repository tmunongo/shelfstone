# Shelfstone

A self-hosted audiobook server. Your books, your server.

## Stack

| Layer      | Tech                        |
|------------|-----------------------------|
| Backend    | Go 1.23                     |
| Templates  | Templ (server-side HTML)    |
| Frontend   | Plain CSS + Alpine.js       |
| Database   | SQLite (single file)        |
| Deployment | Docker + docker-compose     |

## Quick start

```bash
# 1. Copy and edit the compose file
cp docker-compose.yml my-compose.yml
# Edit: set the audiobook volume path, AUTH_USERNAME, AUTH_PASSWORD

# 2. Fetch Alpine.js (if not using CDN)
make fetch-alpine

# 3. Start
docker compose -f my-compose.yml up -d
```

Open http://localhost:8080 and sign in.

## Development

```bash
# Install tooling
go install github.com/a-h/templ/cmd/templ@v0.2.793

# Generate templates + run
make dev
```

After editing any `.templ` file, run `make generate` (or `templ generate`) before restarting.

## Environment variables

| Variable                  | Default              | Description                            |
|---------------------------|----------------------|----------------------------------------|
| `AUDIOBOOK_DATA_DIR`      | `/data/audiobooks`   | Root directory of your audiobook files |
| `DATABASE_PATH`           | `/data/shelfstone.db`| SQLite database path                   |
| `APP_PORT`                | `8080`               | HTTP port                              |
| `AUTH_USERNAME`           | _(empty)_            | Login username                         |
| `AUTH_PASSWORD`           | _(empty)_            | Login password                         |
| `SCAN_INTERVAL_MINUTES`   | `60`                 | Library rescan frequency               |
| `BASE_URL`                | _(empty)_            | Set for reverse proxy (e.g. subdomain) |

## Directory structure expected

```
/data/audiobooks/
  Author Name/
    Book Title/
      cover.jpg        ← optional
      01 - Chapter.mp3
      02 - Chapter.mp3
  Another Author/
    Great Book.m4b     ← single-file M4B works too
```

The scanner reads embedded ID3/MP4 tags first; falls back to directory names.

## Project structure

```
shelfstone/
  cmd/server/          ← main entrypoint + config
  internal/
    auth/              ← session-based auth
    db/                ← SQLite queries and migrations
    metadata/          ← tag extraction (dhowden/tag)
    models/            ← shared data types
    scanner/           ← library directory walker
    server/            ← HTTP router, page handlers, API handlers
  web/
    templates/         ← Templ components (base, home, library, book, listen)
    static/
      css/main.css     ← full design system
      js/
        alpine.min.js  ← Alpine.js (fetch with `make fetch-alpine`)
        app.js         ← player, notes, tags, progress sync
  Dockerfile
  docker-compose.yml
  Makefile
```

## Supported formats

`.mp3` `.m4b` `.m4a` `.ogg` `.opus` `.flac`

## Progress sync

- Position writes to **LocalStorage** every 10 seconds while playing.
- Position syncs to **SQLite** on pause, tab hide, and every 10 seconds.
- On load, the server timestamp is compared against the client's. Newer wins.
- Manual "Force sync" available via the API (`POST /api/progress/:id`).

## Notes on Templ

After editing any `.templ` file you must run:

```bash
templ generate
```

This produces `*_templ.go` files that Go can compile. The `Makefile` runs this
automatically via `make dev` and `make build`.

## License

MIT
