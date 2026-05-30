package models

import "time"

// Audiobook represents a scanned audiobook in the library.
type Audiobook struct {
	ID          int64
	Title       string
	Author      string
	Narrator    string
	Description string
	CoverPath   string // relative path from data dir, or empty
	DurationSec int64
	FilePath    string // directory path relative to data dir
	FileFormat  string // "mp3", "m4b", etc.
	CreatedAt   time.Time
	UpdatedAt   time.Time

	// Derived / joined fields (not stored directly on this row)
	Chapters []Chapter
	Tags     []Tag
	Progress *Progress // may be nil
}

// Chapter is a named segment within an audiobook.
type Chapter struct {
	ID          int64
	AudiobookID int64
	Index       int // zero-based ordering
	Title       string
	StartSec    float64
	EndSec      float64
}

// Progress tracks where a user is in a given audiobook.
type Progress struct {
	ID          int64
	AudiobookID int64
	UserID      int64
	PositionSec float64
	Completed   bool
	UpdatedAt   time.Time
}

// Note is a text annotation attached to an audiobook, optionally to a chapter or timestamp.
type Note struct {
	ID           int64
	AudiobookID  int64
	UserID       int64
	ChapterID    *int64   // nullable
	TimestampSec *float64 // nullable; position within the chapter/book
	Body         string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Tag is a user-defined label that can be applied to audiobooks.
type Tag struct {
	ID   int64
	Name string
}

// User is a local account. Supports multi-user roles ("admin" and "user").
type User struct {
	ID           int64
	Username     string
	PasswordHash string
	Role         string
	CreatedAt    time.Time
}
