package db

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/tmunongo/shelfstone/internal/models"
)

// ---- Audiobooks ----

func UpsertAudiobook(db *sql.DB, a *models.Audiobook) (int64, error) {
	_, err := db.Exec(`
		INSERT INTO audiobooks (title, author, narrator, description, cover_path, duration_sec, file_path, file_format, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))
		ON CONFLICT(file_path) DO UPDATE SET
			title        = excluded.title,
			author       = excluded.author,
			narrator     = excluded.narrator,
			description  = excluded.description,
			cover_path   = excluded.cover_path,
			duration_sec = excluded.duration_sec,
			file_format  = excluded.file_format,
			updated_at   = excluded.updated_at
	`, a.Title, a.Author, a.Narrator, a.Description, a.CoverPath, a.DurationSec, a.FilePath, a.FileFormat)
	if err != nil {
		return 0, fmt.Errorf("upsert audiobook: %w", err)
	}
	var id int64
	err = db.QueryRow(`SELECT id FROM audiobooks WHERE file_path = ?`, a.FilePath).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("get upserted audiobook id: %w", err)
	}
	return id, nil
}

func GetAudiobook(db *sql.DB, id int64) (*models.Audiobook, error) {
	row := db.QueryRow(`
		SELECT id, title, author, narrator, description, cover_path, duration_sec, file_path, file_format, created_at, updated_at
		FROM audiobooks WHERE id = ?`, id)
	return scanAudiobook(row)
}

func ListAudiobooks(db *sql.DB, search string, tagIDs []int64) ([]*models.Audiobook, error) {
	// Simple approach: fetch all, filter in SQL by search term.
	// Tag filtering via a subquery.
	query := `
		SELECT DISTINCT a.id, a.title, a.author, a.narrator, a.description,
			a.cover_path, a.duration_sec, a.file_path, a.file_format, a.created_at, a.updated_at
		FROM audiobooks a`

	args := []any{}

	if len(tagIDs) > 0 {
		query += `
		JOIN audiobook_tags at ON at.audiobook_id = a.id
		WHERE at.tag_id IN (`
		for i, tid := range tagIDs {
			if i > 0 {
				query += ","
			}
			query += "?"
			args = append(args, tid)
		}
		query += ")"
	}

	if search != "" {
		if len(tagIDs) > 0 {
			query += " AND "
		} else {
			query += " WHERE "
		}
		like := "%" + search + "%"
		query += "(a.title LIKE ? OR a.author LIKE ? OR a.narrator LIKE ?)"
		args = append(args, like, like, like)
	}

	query += " ORDER BY a.title ASC"

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list audiobooks: %w", err)
	}
	defer rows.Close()

	var books []*models.Audiobook
	for rows.Next() {
		b, err := scanAudiobookRow(rows)
		if err != nil {
			return nil, err
		}
		books = append(books, b)
	}
	return books, rows.Err()
}

func scanAudiobook(row *sql.Row) (*models.Audiobook, error) {
	var b models.Audiobook
	err := row.Scan(&b.ID, &b.Title, &b.Author, &b.Narrator, &b.Description,
		&b.CoverPath, &b.DurationSec, &b.FilePath, &b.FileFormat, &b.CreatedAt, &b.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("scan audiobook: %w", err)
	}
	return &b, nil
}

func scanAudiobookRow(rows *sql.Rows) (*models.Audiobook, error) {
	var b models.Audiobook
	err := rows.Scan(&b.ID, &b.Title, &b.Author, &b.Narrator, &b.Description,
		&b.CoverPath, &b.DurationSec, &b.FilePath, &b.FileFormat, &b.CreatedAt, &b.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("scan audiobook row: %w", err)
	}
	return &b, nil
}

// ---- Chapters ----

func ReplaceChapters(db *sql.DB, audiobookID int64, chapters []models.Chapter) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM chapters WHERE audiobook_id = ?`, audiobookID); err != nil {
		return fmt.Errorf("delete chapters: %w", err)
	}

	for _, ch := range chapters {
		if _, err := tx.Exec(`
			INSERT INTO chapters (audiobook_id, idx, title, start_sec, end_sec)
			VALUES (?, ?, ?, ?, ?)`,
			audiobookID, ch.Index, ch.Title, ch.StartSec, ch.EndSec,
		); err != nil {
			return fmt.Errorf("insert chapter: %w", err)
		}
	}
	return tx.Commit()
}

func GetChapters(db *sql.DB, audiobookID int64) ([]models.Chapter, error) {
	rows, err := db.Query(`
		SELECT id, audiobook_id, idx, title, start_sec, end_sec
		FROM chapters WHERE audiobook_id = ? ORDER BY idx`, audiobookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var chapters []models.Chapter
	for rows.Next() {
		var ch models.Chapter
		if err := rows.Scan(&ch.ID, &ch.AudiobookID, &ch.Index, &ch.Title, &ch.StartSec, &ch.EndSec); err != nil {
			return nil, err
		}
		chapters = append(chapters, ch)
	}
	return chapters, rows.Err()
}

// ---- Progress ----

func GetProgress(db *sql.DB, audiobookID, userID int64) (*models.Progress, error) {
	var p models.Progress
	err := db.QueryRow(`
		SELECT id, audiobook_id, user_id, position_sec, completed, updated_at
		FROM progress WHERE audiobook_id = ? AND user_id = ?`,
		audiobookID, userID,
	).Scan(&p.ID, &p.AudiobookID, &p.UserID, &p.PositionSec, &p.Completed, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get progress: %w", err)
	}
	return &p, nil
}

func UpsertProgress(db *sql.DB, audiobookID, userID int64, positionSec float64, completed bool) error {
	_, err := db.Exec(`
		INSERT INTO progress (audiobook_id, user_id, position_sec, completed, updated_at)
		VALUES (?, ?, ?, ?, datetime('now'))
		ON CONFLICT(audiobook_id, user_id) DO UPDATE SET
			position_sec = excluded.position_sec,
			completed    = excluded.completed,
			updated_at   = excluded.updated_at
	`, audiobookID, userID, positionSec, completed)
	return err
}

// GetRecentlyPlayed returns the N audiobooks with the most recent progress.
func GetRecentlyPlayed(db *sql.DB, userID int64, limit int) ([]*models.Audiobook, error) {
	rows, err := db.Query(`
		SELECT a.id, a.title, a.author, a.narrator, a.description,
			a.cover_path, a.duration_sec, a.file_path, a.file_format, a.created_at, a.updated_at
		FROM audiobooks a
		JOIN progress p ON p.audiobook_id = a.id
		WHERE p.user_id = ? AND p.completed = 0
		ORDER BY p.updated_at DESC
		LIMIT ?`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var books []*models.Audiobook
	for rows.Next() {
		b, err := scanAudiobookRow(rows)
		if err != nil {
			return nil, err
		}
		books = append(books, b)
	}
	return books, rows.Err()
}

// ---- Notes ----

func CreateNote(db *sql.DB, n *models.Note) (int64, error) {
	res, err := db.Exec(`
		INSERT INTO notes (audiobook_id, user_id, chapter_id, timestamp_sec, body, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, datetime('now'), datetime('now'))`,
		n.AudiobookID, n.UserID, n.ChapterID, n.TimestampSec, n.Body)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func GetNotes(db *sql.DB, audiobookID, userID int64) ([]models.Note, error) {
	rows, err := db.Query(`
		SELECT id, audiobook_id, user_id, chapter_id, timestamp_sec, body, created_at, updated_at
		FROM notes WHERE audiobook_id = ? AND user_id = ? ORDER BY created_at DESC`,
		audiobookID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var notes []models.Note
	for rows.Next() {
		var n models.Note
		if err := rows.Scan(&n.ID, &n.AudiobookID, &n.UserID, &n.ChapterID,
			&n.TimestampSec, &n.Body, &n.CreatedAt, &n.UpdatedAt); err != nil {
			return nil, err
		}
		notes = append(notes, n)
	}
	return notes, rows.Err()
}

func DeleteNote(db *sql.DB, noteID, userID int64) error {
	_, err := db.Exec(`DELETE FROM notes WHERE id = ? AND user_id = ?`, noteID, userID)
	return err
}

// ---- Tags ----

func GetOrCreateTag(db *sql.DB, name string) (int64, error) {
	var id int64
	err := db.QueryRow(`SELECT id FROM tags WHERE name = ?`, name).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		return 0, err
	}
	res, err := db.Exec(`INSERT INTO tags (name) VALUES (?)`, name)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func ListTags(db *sql.DB) ([]models.Tag, error) {
	rows, err := db.Query(`SELECT id, name FROM tags ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []models.Tag
	for rows.Next() {
		var t models.Tag
		if err := rows.Scan(&t.ID, &t.Name); err != nil {
			return nil, err
		}
		tags = append(tags, t)
	}
	return tags, rows.Err()
}

func GetTagsForBook(db *sql.DB, audiobookID int64) ([]models.Tag, error) {
	rows, err := db.Query(`
		SELECT t.id, t.name FROM tags t
		JOIN audiobook_tags at ON at.tag_id = t.id
		WHERE at.audiobook_id = ? ORDER BY t.name`, audiobookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []models.Tag
	for rows.Next() {
		var t models.Tag
		if err := rows.Scan(&t.ID, &t.Name); err != nil {
			return nil, err
		}
		tags = append(tags, t)
	}
	return tags, rows.Err()
}

func SetTagsForBook(db *sql.DB, audiobookID int64, tagNames []string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM audiobook_tags WHERE audiobook_id = ?`, audiobookID); err != nil {
		return err
	}

	for _, name := range tagNames {
		var tagID int64
		err := tx.QueryRow(`SELECT id FROM tags WHERE name = ?`, name).Scan(&tagID)
		if err == sql.ErrNoRows {
			res, err := tx.Exec(`INSERT INTO tags (name) VALUES (?)`, name)
			if err != nil {
				return err
			}
			tagID, _ = res.LastInsertId()
		} else if err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT OR IGNORE INTO audiobook_tags (audiobook_id, tag_id) VALUES (?, ?)`,
			audiobookID, tagID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ---- Progress timestamp reconciliation ----

// ProgressUpdateNewerThan returns true if the DB progress updated_at is newer than clientTime.
// Used to decide whether to override LocalStorage position on load.
func ProgressNewerThan(db *sql.DB, audiobookID, userID int64, clientTime time.Time) (bool, error) {
	var updatedAt time.Time
	err := db.QueryRow(`
		SELECT updated_at FROM progress WHERE audiobook_id = ? AND user_id = ?`,
		audiobookID, userID,
	).Scan(&updatedAt)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return updatedAt.After(clientTime), nil
}
