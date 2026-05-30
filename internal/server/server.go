package server

import (
	"database/sql"
	"net/http"

	"github.com/tmunongo/shelfstone/internal/auth"
	"github.com/tmunongo/shelfstone/internal/scanner"
)

// Config holds the dependencies injected into the server.
type Config struct {
	DB      *sql.DB
	Auth    *auth.Service
	Scanner *scanner.Scanner
	DataDir string
	BaseURL string
}

// New builds and returns the root HTTP handler with all routes registered.
func New(cfg Config) http.Handler {
	h := &handlers{cfg: cfg}
	mux := http.NewServeMux()

	// Static assets
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("web/static"))))

	// PWA routes served at root scope
	mux.HandleFunc("GET /manifest.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/manifest+json")
		http.ServeFile(w, r, "web/static/manifest.json")
	})
	mux.HandleFunc("GET /sw.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		w.Header().Set("Service-Worker-Allowed", "/")
		http.ServeFile(w, r, "web/static/sw.js")
	})

	// Serve cover art and audio files from the data directory (read-only)
	mux.Handle("GET /media/", http.StripPrefix("/media/", http.FileServer(http.Dir(cfg.DataDir))))

	// Auth
	mux.HandleFunc("GET /login", h.loginPage)
	mux.HandleFunc("POST /login", h.loginSubmit)
	mux.HandleFunc("POST /logout", h.logout)

	// App routes (require auth)
	mux.Handle("GET /", h.require(http.HandlerFunc(h.home)))
	mux.Handle("GET /library", h.require(http.HandlerFunc(h.library)))
	mux.Handle("GET /book/{id}", h.require(http.HandlerFunc(h.bookPage)))
	mux.Handle("GET /book/{id}/listen", h.require(http.HandlerFunc(h.listenPage)))
	mux.Handle("GET /offline", h.require(http.HandlerFunc(h.offlinePage)))

	// API — progress sync
	mux.Handle("POST /api/progress/{id}", h.require(http.HandlerFunc(h.apiSaveProgress)))
	mux.Handle("GET /api/progress/{id}", h.require(http.HandlerFunc(h.apiGetProgress)))

	// API — notes
	mux.Handle("POST /api/notes", h.require(http.HandlerFunc(h.apiCreateNote)))
	mux.Handle("DELETE /api/notes/{id}", h.require(http.HandlerFunc(h.apiDeleteNote)))

	// API — tags
	mux.Handle("POST /api/book/{id}/tags", h.require(http.HandlerFunc(h.apiSetTags)))

	// API — metadata edit
	mux.Handle("POST /api/book/{id}/metadata", h.require(http.HandlerFunc(h.apiUpdateMetadata)))

	// API — audio file listing (used by JS player)
	mux.Handle("GET /api/book-files/{id}", h.require(http.HandlerFunc(h.apiBookFiles)))

	// API — library rescan
	mux.Handle("POST /api/scan", h.require(http.HandlerFunc(h.apiTriggerScan)))

	return mux
}

// require is middleware that redirects to /login if the user is not authenticated.
func (h *handlers) require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := h.cfg.Auth.UserFromRequest(r)
		if user == nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		// Store user in request context for downstream handlers.
		ctx := withUser(r.Context(), user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
