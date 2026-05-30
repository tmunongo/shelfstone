package db

import (
	"database/sql"
	"fmt"

	"github.com/tmunongo/shelfstone/internal/models"
)

// AudiobookFull is an alias used by handlers so they only import the db package.
type AudiobookFull = models.Audiobook

// UpdateAudiobookMetadata applies user-supplied metadata overrides.
func UpdateAudiobookMetadata(db *sql.DB, id int64, title, author, narrator, description string) error {
	_, err := db.Exec(`
		UPDATE audiobooks
		SET title = ?, author = ?, narrator = ?, description = ?, updated_at = datetime('now')
		WHERE id = ?`,
		title, author, narrator, description, id,
	)
	if err != nil {
		return fmt.Errorf("update metadata: %w", err)
	}
	return nil
}
