package metadata

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/dhowden/tag"
)

// Meta holds extracted metadata for a single audiobook directory.
type Meta struct {
	Title       string
	Author      string
	Narrator    string
	Description string
	CoverPath   string // relative to dataDir
	DurationSec int64
	Chapters    []ChapterMeta
}

type ChapterMeta struct {
	Title    string
	StartSec float64
	EndSec   float64
}

// Extract attempts to read embedded metadata from the primary audio file in absDir.
func Extract(absDir, relDir, ext string) (*Meta, error) {
	primary, err := findPrimaryFile(absDir, ext)
	if err != nil {
		return nil, err
	}
	relFile := filepath.ToSlash(filepath.Join(relDir, filepath.Base(primary)))
	return ExtractFile(primary, relFile)
}

// ExtractFile reads metadata from a single specific audio file.
// relPath is the path relative to the data directory (used for fallback title/author).
func ExtractFile(absFile, relPath string) (*Meta, error) {
	dir := filepath.Dir(absFile)
	relDir := filepath.Dir(relPath)

	var meta *Meta
	f, err := os.Open(absFile)
	if err == nil {
		defer f.Close()
		m, err := tag.ReadFrom(f)
		if err == nil {
			// For file-level books, prefer embedded title then filename fallback.
			meta = &Meta{
				Title:  coalesce(m.Title(), TitleFromFile(relPath)),
				Author: coalesce(m.Artist(), m.AlbumArtist(), authorFromDir(relDir)),
			}
			if c := m.Composer(); c != "" {
				meta.Narrator = c
			}
			if comment := m.Comment(); comment != "" {
				meta.Description = comment
			}
			if pic := m.Picture(); pic != nil {
				coverPath := filepath.Join(dir, "cover.extracted.jpg")
				if err := writeCoverIfAbsent(coverPath, pic.Data); err == nil {
					meta.CoverPath = filepath.Join(relDir, "cover.extracted.jpg")
				}
			} else {
				meta.CoverPath = findCoverFile(dir, relDir)
			}
		}
	}

	if meta == nil {
		meta = &Meta{
			Title:     TitleFromFile(relPath),
			Author:    authorFromDir(relDir),
			CoverPath: findCoverFile(dir, relDir),
		}
	}

	duration, chapters, err := probeMetadata(absFile)
	if err == nil {
		meta.DurationSec = duration
		meta.Chapters = chapters
	}

	return meta, nil
}

// Fallback builds minimal metadata from the directory structure alone.
func Fallback(absDir, relDir, ext string) *Meta {
	meta := &Meta{
		Title:     titleFromDir(relDir),
		Author:    authorFromDir(relDir),
		CoverPath: findCoverFile(absDir, relDir),
	}

	primary, err := findPrimaryFile(absDir, ext)
	if err == nil {
		duration, chapters, err := probeMetadata(primary)
		if err == nil {
			meta.DurationSec = duration
			meta.Chapters = chapters
		}
	}

	return meta
}

type ffprobeOutput struct {
	Chapters []struct {
		StartTime string `json:"start_time"`
		EndTime   string `json:"end_time"`
		Tags      struct {
			Title string `json:"title"`
		} `json:"tags"`
	} `json:"chapters"`
	Format struct {
		Duration string `json:"duration"`
	} `json:"format"`
}

func probeMetadata(filePath string) (int64, []ChapterMeta, error) {
	cmdPath := findFFprobe()
	cmd := exec.Command(cmdPath, "-v", "error", "-show_entries", "format=duration", "-show_chapters", "-of", "json", filePath)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return 0, nil, fmt.Errorf("ffprobe error: %v, stderr: %s", err, stderr.String())
	}

	var out ffprobeOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		return 0, nil, err
	}

	var durationSec int64
	if out.Format.Duration != "" {
		d, err := strconv.ParseFloat(out.Format.Duration, 64)
		if err == nil {
			durationSec = int64(d)
		}
	}

	var chapters []ChapterMeta
	for _, ch := range out.Chapters {
		start, err1 := strconv.ParseFloat(ch.StartTime, 64)
		end, err2 := strconv.ParseFloat(ch.EndTime, 64)
		if err1 == nil && err2 == nil {
			title := ch.Tags.Title
			if title == "" {
				title = fmt.Sprintf("Chapter %d", len(chapters)+1)
			}
			chapters = append(chapters, ChapterMeta{
				Title:    title,
				StartSec: start,
				EndSec:   end,
			})
		}
	}

	return durationSec, chapters, nil
}

func findFFprobe() string {
	if path, err := exec.LookPath("ffprobe"); err == nil {
		return path
	}
	// Fallback to common Homebrew / Mac paths
	for _, p := range []string{"/opt/homebrew/bin/ffprobe", "/usr/local/bin/ffprobe"} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "ffprobe" // fallback
}

// findPrimaryFile returns the first audio file with the given extension in dir.
func findPrimaryFile(dir, ext string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if !e.IsDir() && strings.ToLower(filepath.Ext(e.Name())) == ext {
			return filepath.Join(dir, e.Name()), nil
		}
	}
	return "", os.ErrNotExist
}

// titleFromDir derives a title from a relative directory path.
// E.g. "Author Name/Book Title" -> "Book Title"
func titleFromDir(relDir string) string {
	parts := strings.Split(filepath.ToSlash(relDir), "/")
	return parts[len(parts)-1]
}

// TitleFromFile derives a human-readable title from a relative file path,
// stripping the extension and trimming common track-number prefixes.
// E.g. "Blinkist/Biography/01 - Steve Jobs.mp3" -> "Steve Jobs"
func TitleFromFile(relFile string) string {
	base := filepath.Base(relFile)
	// Strip extension.
	base = strings.TrimSuffix(base, filepath.Ext(base))
	// Remove leading track numbers like "01 - ", "01. ", "01 "
	for _, sep := range []string{" - ", ". ", " "} {
		idx := strings.Index(base, sep)
		if idx > 0 && idx <= 3 {
			prefix := base[:idx]
			allDigits := true
			for _, c := range prefix {
				if c < '0' || c > '9' {
					allDigits = false
					break
				}
			}
			if allDigits {
				base = strings.TrimSpace(base[idx+len(sep):])
				break
			}
		}
	}
	return base
}

// authorFromDir derives an author from the parent directory name.
func authorFromDir(relDir string) string {
	parts := strings.Split(filepath.ToSlash(relDir), "/")
	if len(parts) >= 2 {
		return parts[len(parts)-2]
	}
	return ""
}

// findCoverFile looks for a cover image in the directory.
func findCoverFile(absDir, relDir string) string {
	candidates := []string{"cover.extracted.jpg", "cover.downloaded.jpg", "cover.jpg", "cover.jpeg", "cover.png", "folder.jpg", "folder.png"}
	for _, name := range candidates {
		if _, err := os.Stat(filepath.Join(absDir, name)); err == nil {
			return filepath.ToSlash(filepath.Join(relDir, name))
		}
	}
	return ""
}

// writeCoverIfAbsent writes image bytes to path only if the file does not already exist.
func writeCoverIfAbsent(path string, data []byte) error {
	if _, err := os.Stat(path); err == nil {
		return nil // already exists
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(data)
	return err
}

func coalesce(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

type openLibrarySearchResponse struct {
	Docs []struct {
		CoverI int      `json:"cover_i"`
		ISBN   []string `json:"isbn"`
	} `json:"docs"`
}

// FetchOnlineCover queries Open Library to search for a book cover by title and author,
// and returns the raw image bytes if found.
func FetchOnlineCover(title, author string) ([]byte, error) {
	if title == "" {
		return nil, fmt.Errorf("empty title")
	}

	// Prepare search query.
	query := url.Values{}
	query.Set("title", title)
	if author != "" {
		query.Set("author", author)
	}
	query.Set("limit", "1")

	searchURL := "https://openlibrary.org/search.json?" + query.Encode()

	req, err := http.NewRequest("GET", searchURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Shelfstone/1.0 (https://github.com/tmunongo/shelfstone)")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search failed with status: %s", resp.Status)
	}

	var searchResult openLibrarySearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResult); err != nil {
		return nil, err
	}

	if len(searchResult.Docs) == 0 {
		return nil, fmt.Errorf("no books found on Open Library")
	}

	doc := searchResult.Docs[0]
	var coverURL string

	if doc.CoverI > 0 {
		coverURL = fmt.Sprintf("https://covers.openlibrary.org/b/id/%d-L.jpg", doc.CoverI)
	} else if len(doc.ISBN) > 0 {
		coverURL = fmt.Sprintf("https://covers.openlibrary.org/b/isbn/%s-L.jpg", doc.ISBN[0])
	} else {
		return nil, fmt.Errorf("no cover art found for book")
	}

	// Download the cover image.
	reqCover, err := http.NewRequest("GET", coverURL, nil)
	if err != nil {
		return nil, err
	}
	reqCover.Header.Set("User-Agent", "Shelfstone/1.0 (https://github.com/tmunongo/shelfstone)")

	respCover, err := client.Do(reqCover)
	if err != nil {
		return nil, err
	}
	defer respCover.Body.Close()

	if respCover.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cover download failed with status: %s", respCover.Status)
	}

	data, err := io.ReadAll(respCover.Body)
	if err != nil {
		return nil, err
	}

	// Double check that we didn't just get a tiny placeholder or blank response.
	// 1x1 pixel empty images are common placeholder responses. A real cover should be > 1KB.
	if len(data) < 1000 {
		return nil, fmt.Errorf("downloaded image is too small to be a cover (%d bytes)", len(data))
	}

	return data, nil
}
