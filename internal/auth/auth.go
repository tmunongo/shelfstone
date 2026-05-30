package auth

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/crypto/bcrypt"

	dbpkg "github.com/tmunongo/shelfstone/internal/db"
	"github.com/tmunongo/shelfstone/internal/models"
)

const sessionCookie = "shelfstone_session"
const sessionDuration = 30 * 24 * time.Hour

// Service handles login, session management, and user lookup.
type Service struct {
	db *sql.DB
}

func New(database *sql.DB, username, password string) *Service {
	s := &Service{db: database}

	// Bootstrap the admin user if credentials are provided and the user doesn't exist.
	if username != "" && password != "" {
		if err := s.ensureUser(username, password); err != nil {
			fmt.Printf("auth: failed to ensure user: %v\n", err)
		}
	}

	// Clean up expired sessions on startup.
	go s.purgeExpiredSessions()

	return s
}

func (s *Service) ensureUser(username, password string) error {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM users WHERE username = ?`, username).Scan(&count)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO users (username, password_hash) VALUES (?, ?)`, username, string(hash))
	return err
}

// Login verifies credentials and returns a session token on success.
func (s *Service) Login(username, password string) (string, error) {
	var user models.User
	err := s.db.QueryRow(`SELECT id, username, password_hash FROM users WHERE username = ?`, username).
		Scan(&user.ID, &user.Username, &user.PasswordHash)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("invalid credentials")
	}
	if err != nil {
		return "", err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return "", fmt.Errorf("invalid credentials")
	}

	token, err := randomToken()
	if err != nil {
		return "", err
	}

	expiresAt := time.Now().Add(sessionDuration)
	_, err = s.db.Exec(
		`INSERT INTO sessions (token, user_id, expires_at) VALUES (?, ?, ?)`,
		token, user.ID, expiresAt.UTC(),
	)
	if err != nil {
		return "", fmt.Errorf("save session: %w", err)
	}

	return token, nil
}

// Logout removes a session.
func (s *Service) Logout(token string) {
	cookie := sessionCookie
	_ = cookie
	s.db.Exec(`DELETE FROM sessions WHERE token = ?`, token)
}

// UserFromRequest extracts and validates the session cookie from a request.
// Returns nil if not authenticated.
func (s *Service) UserFromRequest(r *http.Request) *models.User {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil {
		return nil
	}

	var userID int64
	var expiresAt time.Time
	err = s.db.QueryRow(
		`SELECT user_id, expires_at FROM sessions WHERE token = ?`, cookie.Value,
	).Scan(&userID, &expiresAt)
	if err != nil {
		return nil
	}

	// Check expiry.
	if time.Now().After(expiresAt) {
		s.db.Exec(`DELETE FROM sessions WHERE token = ?`, cookie.Value)
		return nil
	}

	user, err := dbpkg.GetUserByID(s.db, userID)
	if err != nil {
		return nil
	}
	return user
}

func (s *Service) purgeExpiredSessions() {
	s.db.Exec(`DELETE FROM sessions WHERE expires_at < datetime('now')`)
}

// SetSessionCookie writes a session cookie to the response.
func SetSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionDuration.Seconds()),
	})
}

// ClearSessionCookie removes the session cookie.
func ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:   sessionCookie,
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
}

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
