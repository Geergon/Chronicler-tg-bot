package database

import (
	"database/sql"
	"fmt"
)

type ChatPackInfo struct {
	Name      string
	Title     string
	PackIndex int
}

func GetCreatorIDFromDB(db *sql.DB, chatID int64) (int64, bool) {
	var id int64
	err := db.QueryRow(
		`SELECT creator_id FROM chat_stickers WHERE chat_id = ? AND creator_id != 0`,
		chatID,
	).Scan(&id)
	return id, err == nil && id != 0
}

func GetOrCreatePackInfo(db *sql.DB, chatID int64, botUsername, chatName string) (ChatPackInfo, error) {
	absID := chatID
	if absID < 0 {
		absID = -absID
	}

	var info ChatPackInfo
	query := `SELECT current_pack_name, current_pack_title, pack_index FROM chat_stickers WHERE chat_id = ?`
	err := db.QueryRow(query, chatID).Scan(&info.Name, &info.Title, &info.PackIndex)

	if err == sql.ErrNoRows {
		info.PackIndex = 1
		info.Name = fmt.Sprintf("quotes_%d_v%d_by_%s", absID, info.PackIndex, botUsername)
		info.Title = fmt.Sprintf("%s | v%d pack by @%s", chatName, info.PackIndex, botUsername)

		if err := SaveOrUpdatePackInfo(db, chatID, info); err != nil {
			return info, fmt.Errorf("saveOrUpdatePackInfo on create: %w", err)
		}
		return info, nil
	}

	if err != nil {
		return info, fmt.Errorf("query pack info: %w", err)
	}

	return info, nil
}

func GetRandomQuote(db *sql.DB, chatID int64) (string, error) {
	var fileID string
	err := db.QueryRow(`
        SELECT file_id FROM quotes
        WHERE chat_id = ?
        ORDER BY RANDOM()
        LIMIT 1`,
		chatID,
	).Scan(&fileID)
	return fileID, err
}

func GetRandomQuoteByAuthor(db *sql.DB, chatID, savedBy int64) (string, error) {
	var fileID string
	err := db.QueryRow(`
        SELECT file_id FROM quotes
        WHERE chat_id = ? AND saved_by = ?
        ORDER BY RANDOM()
        LIMIT 1`,
		chatID, savedBy,
	).Scan(&fileID)
	return fileID, err
}
