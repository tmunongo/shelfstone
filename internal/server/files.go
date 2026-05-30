package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	dbpkg "github.com/tmunongo/shelfstone/internal/db"
)

// apiBookFiles returns the ordered list of audio file paths for a book,
// relative to the data directory. The JS player uses this to resolve the
// audio source URL.
func init() {
	// This handler is registered in server.go; defined here to keep API handlers together.
}

func (h *handlers) apiBookFiles(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}

	book, err := dbpkg.GetAudiobook(h.cfg.DB, id)
	if err != nil || book == nil {
		jsonError(w, "not found", http.StatusNotFound)
		return
	}

	absDir := filepath.Join(h.cfg.DataDir, book.FilePath)
	entries, err := os.ReadDir(absDir)
	if err != nil {
		jsonError(w, "cannot read directory", http.StatusInternalServerError)
		return
	}

	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		switch ext {
		case ".mp3", ".m4b", ".m4a", ".ogg", ".opus", ".flac":
			// Store relative to data dir so it can be served under /media/
			rel := filepath.Join(book.FilePath, e.Name())
			files = append(files, filepath.ToSlash(rel))
		}
	}

	// Natural sort so Part 1, Part 2 ... Part 10 comes out correctly.
	sort.Strings(files)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"files": files})
}
