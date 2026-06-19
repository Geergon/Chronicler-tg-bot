package database

import (
	"database/sql"
	"fmt"
)

func SaveCreatorID(db *sql.DB, chatID, creatorID int64) error {
	_, err := db.Exec(
		`UPDATE chat_stickers SET creator_id = ? WHERE chat_id = ?`,
		creatorID, chatID,
	)
	return err
}

func SaveOrUpdatePackInfo(db *sql.DB, chatID int64, info ChatPackInfo) error {
	query := `
		INSERT INTO chat_stickers (chat_id, current_pack_name, current_pack_title, pack_index)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(chat_id) DO UPDATE SET
			current_pack_name  = excluded.current_pack_name,
			current_pack_title = excluded.current_pack_title,
			pack_index         = excluded.pack_index
	`
	_, err := db.Exec(query, chatID, info.Name, info.Title, info.PackIndex)
	if err != nil {
		return fmt.Errorf("saveOrUpdatePackInfo: %w", err)
	}
	return nil
}
