package db

import (
	"database/sql"
	"fmt"

	"github.com/tmunongo/shelfstone/internal/models"
)

func GetUserByID(db *sql.DB, id int64) (*models.User, error) {
	var u models.User
	err := db.QueryRow(`SELECT id, username, password_hash, role, created_at FROM users WHERE id = ?`, id).
		Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get user by id: %w", err)
	}
	return &u, nil
}
