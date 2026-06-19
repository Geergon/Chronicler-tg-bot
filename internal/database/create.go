package database

import "database/sql"

func InitDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	createTable := `
        CREATE TABLE IF NOT EXISTS chat_stickers (
						chat_id INTEGER PRIMARY KEY, 
						current_pack_name TEXT NOT NULL,
						current_pack_title TEXT NOT NULL,
						pack_index INTEGER NOT NULL DEFAULT 1
				);
    `
	_, err = db.Exec(createTable)
	if err != nil {
		return nil, err
	}

	return db, nil
}
