package db

import (
	"database/sql"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
)

// Open opens (or creates) the SQLite database at path and sets pragmas for
// good performance and reliability on a single-writer workload.
func Open(path string) (*sql.DB, error) {
	// The DSN enables WAL mode and foreign keys at the driver level.
	dsn := fmt.Sprintf("file:%s?_journal_mode=WAL&_foreign_keys=on&_busy_timeout=5000", path)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// SQLite works best with a single writer connection.
	db.SetMaxOpenConns(1)

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	return db, nil
}

// Migrate runs all schema migrations idempotently.
// We use a simple sequential versioning approach — no external migration library needed.
func Migrate(db *sql.DB) error {
	if _, err := db.Exec(createSchemaVersionTable); err != nil {
		return fmt.Errorf("create schema_versions: %w", err)
	}

	for _, m := range migrations {
		var exists bool
		err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM schema_versions WHERE version = ?)`, m.version).Scan(&exists)
		if err != nil {
			return fmt.Errorf("check version %d: %w", m.version, err)
		}
		if exists {
			continue
		}

		if _, err := db.Exec(m.sql); err != nil {
			return fmt.Errorf("apply migration %d: %w", m.version, err)
		}
		if _, err := db.Exec(`INSERT INTO schema_versions (version) VALUES (?)`, m.version); err != nil {
			return fmt.Errorf("record migration %d: %w", m.version, err)
		}
	}
	return nil
}

const createSchemaVersionTable = `
CREATE TABLE IF NOT EXISTS schema_versions (
	version   INTEGER PRIMARY KEY,
	applied_at DATETIME DEFAULT (datetime('now'))
);`

type migration struct {
	version int
	sql     string
}

var migrations = []migration{
	{1, migration1},
	{2, migration2},
	{3, migration3},
}

// migration3 adds a role column to the users table.
const migration3 = `
ALTER TABLE users ADD COLUMN role TEXT NOT NULL DEFAULT 'user';
`

// migration2 adds a persistent sessions table so logins survive server restarts.
const migration2 = `
CREATE TABLE IF NOT EXISTS sessions (
	token      TEXT PRIMARY KEY,
	user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	expires_at DATETIME NOT NULL,
	created_at DATETIME DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id);
`

// migration1 creates the full initial schema.
const migration1 = `
CREATE TABLE IF NOT EXISTS users (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	username      TEXT NOT NULL UNIQUE,
	password_hash TEXT NOT NULL,
	created_at    DATETIME DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS audiobooks (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	title        TEXT NOT NULL,
	author       TEXT NOT NULL DEFAULT '',
	narrator     TEXT NOT NULL DEFAULT '',
	description  TEXT NOT NULL DEFAULT '',
	cover_path   TEXT NOT NULL DEFAULT '',
	duration_sec INTEGER NOT NULL DEFAULT 0,
	file_path    TEXT NOT NULL UNIQUE,
	file_format  TEXT NOT NULL DEFAULT '',
	created_at   DATETIME DEFAULT (datetime('now')),
	updated_at   DATETIME DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS chapters (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	audiobook_id INTEGER NOT NULL REFERENCES audiobooks(id) ON DELETE CASCADE,
	idx          INTEGER NOT NULL,
	title        TEXT NOT NULL DEFAULT '',
	start_sec    REAL NOT NULL DEFAULT 0,
	end_sec      REAL NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_chapters_audiobook ON chapters(audiobook_id, idx);

CREATE TABLE IF NOT EXISTS progress (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	audiobook_id INTEGER NOT NULL REFERENCES audiobooks(id) ON DELETE CASCADE,
	user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	position_sec REAL NOT NULL DEFAULT 0,
	completed    INTEGER NOT NULL DEFAULT 0,
	updated_at   DATETIME DEFAULT (datetime('now')),
	UNIQUE(audiobook_id, user_id)
);

CREATE TABLE IF NOT EXISTS notes (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	audiobook_id  INTEGER NOT NULL REFERENCES audiobooks(id) ON DELETE CASCADE,
	user_id       INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	chapter_id    INTEGER REFERENCES chapters(id) ON DELETE SET NULL,
	timestamp_sec REAL,
	body          TEXT NOT NULL DEFAULT '',
	created_at    DATETIME DEFAULT (datetime('now')),
	updated_at    DATETIME DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS tags (
	id   INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL UNIQUE
);

CREATE TABLE IF NOT EXISTS audiobook_tags (
	audiobook_id INTEGER NOT NULL REFERENCES audiobooks(id) ON DELETE CASCADE,
	tag_id       INTEGER NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
	PRIMARY KEY (audiobook_id, tag_id)
);
`
