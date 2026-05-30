package scanner

import (
	"context"
	"database/sql"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/tmunongo/shelfstone/internal/db"
	"github.com/tmunongo/shelfstone/internal/metadata"
	"github.com/tmunongo/shelfstone/internal/models"
)

// Scanner walks the audiobook directory and synchronises it with the database.
type Scanner struct {
	db      *sql.DB
	dataDir string
	mu      sync.Mutex // prevents concurrent scans
}

func New(database *sql.DB, dataDir string) *Scanner {
	return &Scanner{db: database, dataDir: dataDir}
}

// Scan walks dataDir, finds audiobook directories, and upserts their metadata.
func (s *Scanner) Scan(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return filepath.WalkDir(s.dataDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			log.Printf("scanner: walk error at %s: %v", path, err)
			return nil // keep walking
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if d.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if !isSupportedAudio(ext) {
			return nil
		}

		// Only process the first audio file per directory to avoid
		// creating duplicate entries for multi-file books.
		dir := filepath.Dir(path)
		rel, err := filepath.Rel(s.dataDir, dir)
		if err != nil {
			return nil
		}

		if err := s.processBook(ctx, dir, rel, ext); err != nil {
			log.Printf("scanner: error processing %s: %v", rel, err)
		}

		// Skip the rest of this directory; WalkDir will continue into subdirs.
		return filepath.SkipDir
	})
}

func (s *Scanner) processBook(ctx context.Context, absDir, relDir, ext string) error {
	meta, err := metadata.Extract(absDir, relDir, ext)
	if err != nil {
		log.Printf("scanner: metadata extraction failed for %s: %v", relDir, err)
		// Use fallback
		meta = metadata.Fallback(absDir, relDir, ext)
	}

	book := &models.Audiobook{
		Title:       meta.Title,
		Author:      meta.Author,
		Narrator:    meta.Narrator,
		Description: meta.Description,
		CoverPath:   meta.CoverPath,
		DurationSec: meta.DurationSec,
		FilePath:    relDir,
		FileFormat:  strings.TrimPrefix(ext, "."),
	}

	id, err := db.UpsertAudiobook(s.db, book)
	if err != nil {
		return err
	}

	log.Printf("scanner: processed book %s (ID %d): duration=%ds, chapters=%d", relDir, id, meta.DurationSec, len(meta.Chapters))

	if len(meta.Chapters) > 0 {
		chapters := make([]models.Chapter, len(meta.Chapters))
		for i, ch := range meta.Chapters {
			chapters[i] = models.Chapter{
				AudiobookID: id,
				Index:       i,
				Title:       ch.Title,
				StartSec:    ch.StartSec,
				EndSec:      ch.EndSec,
			}
		}
		if err := db.ReplaceChapters(s.db, id, chapters); err != nil {
			log.Printf("scanner: chapter replace error for %s: %v", relDir, err)
		}
	}

	return nil
}

func isSupportedAudio(ext string) bool {
	switch ext {
	case ".mp3", ".m4b", ".m4a", ".ogg", ".opus", ".flac":
		return true
	}
	return false
}
