package scanner

import (
	"context"
	"database/sql"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

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

// Scan walks dataDir recursively, finds every directory that directly
// contains at least one audio file, and upserts it as a book.  Books can be
// nested arbitrarily deep (e.g. series/volume/disc/audio.mp3).
//
// Directories with a single audio file are treated as one book (typical for
// M4B files). Directories with multiple audio files create one book entry
// per file (e.g. Blinkist summaries, where each MP3 is a separate title).
func (s *Scanner) Scan(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// processedDirs tracks directories already analyzed so we only do the
	// counting and processing once per directory regardless of file count.
	processedDirs := make(map[string]bool)

	return filepath.WalkDir(s.dataDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			log.Printf("scanner: walk error at %s: %v", path, err)
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if d.IsDir() {
			return nil // descend into every directory
		}

		ext := strings.ToLower(filepath.Ext(path))
		if !isSupportedAudio(ext) {
			return nil
		}

		dir := filepath.Dir(path)
		rel, err := filepath.Rel(s.dataDir, dir)
		if err != nil || rel == "." {
			return nil // skip files at data-dir root
		}

		if processedDirs[rel] {
			return nil // already handled this directory
		}
		processedDirs[rel] = true

		// Count audio files in this directory to decide the strategy.
		audioFiles := collectAudioFiles(dir)

		if len(audioFiles) <= 1 {
			// Single audio file (or none found somehow) — directory = one book.
			if err := s.processBook(ctx, dir, rel, ext); err != nil {
				log.Printf("scanner: error processing %s: %v", rel, err)
			}
		} else {
			// Multiple files — each file is its own standalone book.
			for _, name := range audioFiles {
				absFile := filepath.Join(dir, name)
				relFile := filepath.ToSlash(filepath.Join(rel, name))
				if err := s.processBookFile(ctx, absFile, relFile); err != nil {
					log.Printf("scanner: error processing file %s: %v", relFile, err)
				}
			}
		}

		return nil // never SkipDir — keep walking for deeper books
	})
}

// collectAudioFiles returns a sorted list of audio filenames in dir.
func collectAudioFiles(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && isSupportedAudio(strings.ToLower(filepath.Ext(e.Name()))) {
			names = append(names, e.Name())
		}
	}
	return names
}

func (s *Scanner) processBook(ctx context.Context, absDir, relDir, ext string) error {
	meta, err := metadata.Extract(absDir, relDir, ext)
	if err != nil {
		log.Printf("scanner: metadata extraction failed for %s: %v", relDir, err)
		meta = metadata.Fallback(absDir, relDir, ext)
	}

	if meta.CoverPath == "" {
		meta.CoverPath = s.fetchAndSaveOnlineCover(absDir, relDir, meta.Title, meta.Author)
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

	return s.saveChapters(id, relDir, meta.Chapters)
}

// processBookFile handles a single audio file that should be its own library entry.
// relFile is like "Blinkist/Biography/01 - Steve Jobs.mp3".
func (s *Scanner) processBookFile(ctx context.Context, absFile, relFile string) error {
	ext := strings.ToLower(filepath.Ext(absFile))
	meta, err := metadata.ExtractFile(absFile, relFile)
	if err != nil {
		log.Printf("scanner: metadata extraction failed for %s: %v", relFile, err)
		dir := filepath.Dir(absFile)
		relDir := filepath.Dir(relFile)
		meta = metadata.Fallback(dir, relDir, ext)
		meta.Title = metadata.TitleFromFile(relFile)
	}

	if meta.CoverPath == "" {
		dir := filepath.Dir(absFile)
		relDir := filepath.Dir(relFile)
		meta.CoverPath = s.fetchAndSaveOnlineCover(dir, relDir, meta.Title, meta.Author)
	}

	book := &models.Audiobook{
		Title:       meta.Title,
		Author:      meta.Author,
		Narrator:    meta.Narrator,
		Description: meta.Description,
		CoverPath:   meta.CoverPath,
		DurationSec: meta.DurationSec,
		FilePath:    relFile, // store the file path, not just the directory
		FileFormat:  strings.TrimPrefix(ext, "."),
	}

	id, err := db.UpsertAudiobook(s.db, book)
	if err != nil {
		return err
	}

	return s.saveChapters(id, relFile, meta.Chapters)
}

func (s *Scanner) saveChapters(id int64, label string, chapters []metadata.ChapterMeta) error {
	if len(chapters) == 0 {
		return nil
	}
	dbChapters := make([]models.Chapter, len(chapters))
	for i, ch := range chapters {
		dbChapters[i] = models.Chapter{
			AudiobookID: id,
			Index:       i,
			Title:       ch.Title,
			StartSec:    ch.StartSec,
			EndSec:      ch.EndSec,
		}
	}
	if err := db.ReplaceChapters(s.db, id, dbChapters); err != nil {
		log.Printf("scanner: chapter replace error for %s: %v", label, err)
	}
	return nil
}

func (s *Scanner) fetchAndSaveOnlineCover(absDir, relDir, title, author string) string {
	// Check if there is already a downloaded cover
	downloadedPath := filepath.Join(absDir, "cover.downloaded.jpg")
	if _, err := os.Stat(downloadedPath); err == nil {
		return filepath.ToSlash(filepath.Join(relDir, "cover.downloaded.jpg"))
	}

	// Attempt fetch
	log.Printf("scanner: fetching online cover for %q by %q...", title, author)
	data, err := metadata.FetchOnlineCover(title, author)
	if err != nil {
		// Log the error but keep going silently
		return ""
	}

	// Save
	if err := os.WriteFile(downloadedPath, data, 0644); err != nil {
		log.Printf("scanner: failed to save online cover to %s: %v", downloadedPath, err)
		return ""
	}

	log.Printf("scanner: successfully downloaded cover for %q", title)
	// Sleep 1 second to respect Open Library's API rate limits
	time.Sleep(1 * time.Second)

	return filepath.ToSlash(filepath.Join(relDir, "cover.downloaded.jpg"))
}

func isSupportedAudio(ext string) bool {
	switch ext {
	case ".mp3", ".m4b", ".m4a", ".ogg", ".opus", ".flac":
		return true
	}
	return false
}
