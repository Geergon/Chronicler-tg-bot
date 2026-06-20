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
						pack_index INTEGER NOT NULL DEFAULT 0,
						creator_id INTEGER NOT NULL DEFAULT 0
				);
    `
	_, err = db.Exec(createTable)
	if err != nil {
		return nil, err
	}

	return db, nil
}

func InitQuotesDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	createTable := `
CREATE TABLE IF NOT EXISTS quotes (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    chat_id     INTEGER NOT NULL,
    file_id     TEXT    NOT NULL,
    created_at  INTEGER NOT NULL DEFAULT (unixepoch()),
    saved_by    INTEGER NOT NULL DEFAULT 0
);`
	_, err = db.Exec(createTable)
	if err != nil {
		return nil, err
	}

	return db, nil
}
