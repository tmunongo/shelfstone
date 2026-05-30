package server

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"

	"github.com/tmunongo/shelfstone/internal/auth"
	dbpkg "github.com/tmunongo/shelfstone/internal/db"
	"github.com/tmunongo/shelfstone/web/templates"
)

type handlers struct {
	cfg Config
}

// ---- Auth ----

func (h *handlers) loginPage(w http.ResponseWriter, r *http.Request) {
	templates.LoginPage("").Render(r.Context(), w)
}

func (h *handlers) loginSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	username := r.FormValue("username")
	password := r.FormValue("password")

	token, err := h.cfg.Auth.Login(username, password)
	if err != nil {
		templates.LoginPage("Invalid username or password.").Render(r.Context(), w)
		return
	}

	auth.SetSessionCookie(w, token)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *handlers) logout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("shelfstone_session")
	if err == nil {
		h.cfg.Auth.Logout(cookie.Value)
	}
	auth.ClearSessionCookie(w)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// ---- Home ----

func (h *handlers) home(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())

	recent, err := dbpkg.GetRecentlyPlayed(h.cfg.DB, user.ID, 6)
	if err != nil {
		h.internalError(w, err)
		return
	}

	// Load progress for each recent book
	for _, b := range recent {
		prog, _ := dbpkg.GetProgress(h.cfg.DB, b.ID, user.ID)
		b.Progress = prog
	}

	templates.HomePage(user, recent).Render(r.Context(), w)
}

// ---- Library ----

func (h *handlers) library(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())

	search := r.URL.Query().Get("q")
	// Tag filtering: ?tag=1&tag=2
	tagParams := r.URL.Query()["tag"]
	var tagIDs []int64
	for _, tp := range tagParams {
		id, err := strconv.ParseInt(tp, 10, 64)
		if err == nil {
			tagIDs = append(tagIDs, id)
		}
	}

	sortBy := r.URL.Query().Get("sort")
	if sortBy == "" {
		sortBy = "title_asc"
	}

	pageStr := r.URL.Query().Get("page")
	page, err := strconv.Atoi(pageStr)
	if err != nil || page < 1 {
		page = 1
	}

	limit := 24
	offset := (page - 1) * limit

	books, totalCount, err := dbpkg.ListAudiobooks(h.cfg.DB, search, tagIDs, sortBy, limit, offset)
	if err != nil {
		h.internalError(w, err)
		return
	}

	for _, b := range books {
		prog, _ := dbpkg.GetProgress(h.cfg.DB, b.ID, user.ID)
		b.Progress = prog
	}

	allTags, err := dbpkg.ListTags(h.cfg.DB)
	if err != nil {
		h.internalError(w, err)
		return
	}

	totalPages := (totalCount + limit - 1) / limit
	if totalPages < 1 {
		totalPages = 1
	}

	templates.LibraryPage(user, books, allTags, search, tagIDs, sortBy, page, totalPages).Render(r.Context(), w)
}

// ---- Book ----

func (h *handlers) bookPage(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	book, err := h.loadBook(r)
	if err != nil {
		h.notFound(w)
		return
	}

	chapters, err := dbpkg.GetChapters(h.cfg.DB, book.ID)
	if err != nil {
		h.internalError(w, err)
		return
	}
	book.Chapters = chapters

	notes, err := dbpkg.GetNotes(h.cfg.DB, book.ID, user.ID)
	if err != nil {
		h.internalError(w, err)
		return
	}

	tags, err := dbpkg.GetTagsForBook(h.cfg.DB, book.ID)
	if err != nil {
		h.internalError(w, err)
		return
	}
	book.Tags = tags

	prog, _ := dbpkg.GetProgress(h.cfg.DB, book.ID, user.ID)
	book.Progress = prog

	templates.BookPage(user, book, notes).Render(r.Context(), w)
}

// ---- Listen ----

func (h *handlers) listenPage(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	book, err := h.loadBook(r)
	if err != nil {
		h.notFound(w)
		return
	}

	chapters, err := dbpkg.GetChapters(h.cfg.DB, book.ID)
	if err != nil {
		h.internalError(w, err)
		return
	}
	book.Chapters = chapters

	prog, _ := dbpkg.GetProgress(h.cfg.DB, book.ID, user.ID)
	book.Progress = prog

	templates.ListenPage(user, book).Render(r.Context(), w)
}

// ---- Helpers ----

func (h *handlers) loadBook(r *http.Request) (*dbpkg.AudiobookFull, error) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid id")
	}
	book, err := dbpkg.GetAudiobook(h.cfg.DB, id)
	if err == sql.ErrNoRows || book == nil {
		return nil, fmt.Errorf("not found")
	}
	return book, err
}

func (h *handlers) internalError(w http.ResponseWriter, err error) {
	fmt.Printf("internal error: %v\n", err)
	http.Error(w, "internal server error", http.StatusInternalServerError)
}

func (h *handlers) notFound(w http.ResponseWriter) {
	http.Error(w, "not found", http.StatusNotFound)
}
