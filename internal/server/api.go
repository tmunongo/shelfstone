package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	dbpkg "github.com/tmunongo/shelfstone/internal/db"
	"github.com/tmunongo/shelfstone/internal/models"
)

// ---- Progress ----

type progressPayload struct {
	PositionSec float64 `json:"position_sec"`
	Completed   bool    `json:"completed"`
	ClientTime  string  `json:"client_time"` // RFC3339; used for conflict resolution
}

func (h *handlers) apiSaveProgress(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}

	var p progressPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		jsonError(w, "bad json", http.StatusBadRequest)
		return
	}

	if err := dbpkg.UpsertProgress(h.cfg.DB, id, user.ID, p.PositionSec, p.Completed); err != nil {
		jsonError(w, "db error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *handlers) apiGetProgress(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}

	// Compare with client's LocalStorage timestamp if provided.
	clientTimeStr := r.URL.Query().Get("client_time")
	if clientTimeStr != "" {
		clientTime, err := time.Parse(time.RFC3339, clientTimeStr)
		if err == nil {
			newer, _ := dbpkg.ProgressNewerThan(h.cfg.DB, id, user.ID, clientTime)
			if !newer {
				// Client is ahead; tell it to keep its own value.
				jsonOK(w, map[string]any{"use_local": true})
				return
			}
		}
	}

	prog, err := dbpkg.GetProgress(h.cfg.DB, id, user.ID)
	if err != nil {
		jsonError(w, "db error", http.StatusInternalServerError)
		return
	}
	if prog == nil {
		jsonOK(w, map[string]any{"position_sec": 0, "completed": false})
		return
	}
	jsonOK(w, map[string]any{
		"position_sec": prog.PositionSec,
		"completed":    prog.Completed,
		"updated_at":   prog.UpdatedAt.Format(time.RFC3339),
	})
}

// ---- Notes ----

type notePayload struct {
	AudiobookID  int64    `json:"audiobook_id"`
	ChapterID    *int64   `json:"chapter_id,omitempty"`
	TimestampSec *float64 `json:"timestamp_sec,omitempty"`
	Body         string   `json:"body"`
}

func (h *handlers) apiCreateNote(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	var p notePayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		jsonError(w, "bad json", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(p.Body) == "" {
		jsonError(w, "body required", http.StatusBadRequest)
		return
	}

	note := &models.Note{
		AudiobookID:  p.AudiobookID,
		UserID:       user.ID,
		ChapterID:    p.ChapterID,
		TimestampSec: p.TimestampSec,
		Body:         p.Body,
	}

	id, err := dbpkg.CreateNote(h.cfg.DB, note)
	if err != nil {
		jsonError(w, "db error", http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]any{"id": id})
}

func (h *handlers) apiDeleteNote(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := dbpkg.DeleteNote(h.cfg.DB, id, user.ID); err != nil {
		jsonError(w, "db error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- Tags ----

type tagsPayload struct {
	Tags []string `json:"tags"`
}

func (h *handlers) apiSetTags(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	var p tagsPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		jsonError(w, "bad json", http.StatusBadRequest)
		return
	}
	if err := dbpkg.SetTagsForBook(h.cfg.DB, id, p.Tags); err != nil {
		jsonError(w, "db error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- Metadata edit ----

type metadataPayload struct {
	Title       string `json:"title"`
	Author      string `json:"author"`
	Narrator    string `json:"narrator"`
	Description string `json:"description"`
}

func (h *handlers) apiUpdateMetadata(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	var p metadataPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		jsonError(w, "bad json", http.StatusBadRequest)
		return
	}
	if err := dbpkg.UpdateAudiobookMetadata(h.cfg.DB, id, p.Title, p.Author, p.Narrator, p.Description); err != nil {
		jsonError(w, "db error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- Manual scan ----

func (h *handlers) apiTriggerScan(w http.ResponseWriter, r *http.Request) {
	go func() {
		if err := h.cfg.Scanner.Scan(context.Background()); err != nil {
			// errors are logged inside Scan
			_ = err
		}
	}()
	jsonOK(w, map[string]any{"status": "scan started"})
}

// ---- Helpers ----

func pathID(r *http.Request, key string) (int64, error) {
	return strconv.ParseInt(r.PathValue(key), 10, 64)
}

func jsonOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
