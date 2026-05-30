# Shelfstone

[![Publish Docker Image](https://github.com/tmunongo/shelfstone/actions/workflows/docker-publish.yml/badge.svg)](https://github.com/tmunongo/shelfstone/actions/workflows/docker-publish.yml)

A self-hosted audiobook server. Your books, your server.

## Stack

| Layer      | Tech                                                    |
|------------|---------------------------------------------------------|
| Backend    | Go 1.26                                                 |
| Templates  | Templ 0.3 (server-side HTML compilation)                |
| Frontend   | Plain CSS + Alpine.js                                   |
| Database   | SQLite (with WAL mode enabling fast reads/writes)       |
| Extraction | ffmpeg / ffprobe (precise durations & chapters parsing) |
| Deployment | Docker + Docker Compose (published at GHCR)            |

## Run with Docker

You can run Shelfstone directly using the pre-built container from the GitHub Container Registry (GHCR):

```bash
docker run -d \
  --name shelfstone \
  --restart unless-stopped \
  -p 8080:8080 \
  -v /absolute/path/to/your/audiobooks:/data/audiobooks:ro \
  -v shelfstone_data:/data \
  -e AUTH_USERNAME=admin \
  -e AUTH_PASSWORD=password123 \
  ghcr.io/tmunongo/shelfstone:latest
```

Open `http://localhost:8080` and log in with your credentials.

## Run with Docker Compose

Here is a ready-to-use Docker Compose snippet (`compose.yml`):

```yaml
services:
  shelfstone:
    image: ghcr.io/tmunongo/shelfstone:latest
    container_name: shelfstone
    restart: unless-stopped
    ports:
      - "8080:8080"
    volumes:
      - /path/to/your/audiobooks:/data/audiobooks:ro # read-only audiobook source mount
      - shelfstone_data:/data # persists the database & sessions
    environment:
      AUDIOBOOK_DATA_DIR: /data/audiobooks
      DATABASE_PATH: /data/shelfstone.db
      APP_PORT: 8080
      AUTH_USERNAME: admin
      AUTH_PASSWORD: changeme # change this password
      SCAN_INTERVAL_MINUTES: 60
      # BASE_URL: https://audiobooks.yourdomain.com  # uncomment if behind reverse proxy

volumes:
  shelfstone_data:
```

## Quick Start (Development)

To build and run Shelfstone locally for development:

```bash
# 1. Install Templ compiler tooling
go install github.com/a-h/templ/cmd/templ@latest

# 2. Fetch Alpine.js assets locally
make fetch-alpine

# 3. Generate Go template files and start live reload dev server
make dev
```

After editing any `.templ` template, run `make generate` (or `templ generate`) to rebuild elements before compilation.

## Environment Variables

| Variable                  | Default               | Description                            |
|---------------------------|-----------------------|----------------------------------------|
| `AUDIOBOOK_DATA_DIR`      | `/data/audiobooks`    | Root directory of your audiobook files |
| `DATABASE_PATH`           | `/data/shelfstone.db` | SQLite database file location          |
| `APP_PORT`                | `8080`                | HTTP listening port                    |
| `AUTH_USERNAME`           | _(empty)_             | Login username (required)              |
| `AUTH_PASSWORD`           | _(empty)_             | Login password (required)              |
| `SCAN_INTERVAL_MINUTES`   | `60`                  | Library rescan frequency               |
| `BASE_URL`                | _(empty)_             | Set for reverse proxy setups           |

## Expected Directory Structure

```
/data/audiobooks/
  Author Name/
    Book Title/
      cover.jpg        ← optional local cover
      01 - Chapter.mp3
      02 - Chapter.mp3
  Another Author/
    Great Book.m4b     ← single-file M4B formats are fully supported
```

> [!TIP]
> **Dynamic Cover Fetching**: If local cover art (`cover.jpg`, `cover.png`, etc.) is missing, Shelfstone will automatically search for the book on the Open Library API and download the correct high-quality cover in the background!

## Project Structure

```
shelfstone/
  cmd/server/          ← Main server entrypoint and config parser
  internal/
    auth/              ← Session-based authentication flow
    db/                ← SQLite queries and idempotent version migrations
    metadata/          ← Tag extraction (dhowden/tag) & ffprobe duration / chapters
    models/            ← Shared domain types & structs
    scanner/           ← Library multi-file directory watcher & online cover fetcher
    server/            ← HTTP router, session middleware, page and API handlers
  web/
    templates/         ← Templ UI components (base, home, library, book, listen)
    static/
      css/main.css     ← Beautiful, responsive custom design system
      js/
        alpine.min.js  ← Alpine.js (local build)
        app.js         ← Player lifecycle, notes, tag editor, position synchronization
  Dockerfile
  compose.yml
  Makefile
```

## Supported Audio Formats

- `.mp3`, `.m4b`, `.m4a`, `.ogg`, `.opus`, `.flac`

## Position Synchronization

- Position details are saved to **LocalStorage** every 10 seconds during playback.
- Position is synchronized to the **SQLite database** on pausing, navigating away, tab hiding, and every 10 seconds.
- On initialization, client-side timestamps are compared against server-side progress records. The newest timestamp wins.

## License

MIT
