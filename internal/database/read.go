package database

import (
	"database/sql"
	"fmt"

	"github.com/celestix/gotgproto/ext"
	"github.com/gotd/td/tg"
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

func GetCreatorID(ctx *ext.Context, chatID int64) (int64, error) {
	inputPeer := ctx.PeerStorage.GetInputPeerById(chatID)
	inputChannel, ok := inputPeer.(*tg.InputPeerChannel)
	if !ok {
		return 0, fmt.Errorf("getCreatorID: it's not a channel or supergroup")
	}

	res, err := ctx.Raw.ChannelsGetParticipants(ctx, &tg.ChannelsGetParticipantsRequest{
		Channel: &tg.InputChannel{
			ChannelID:  inputChannel.ChannelID,
			AccessHash: inputChannel.AccessHash,
		},
		Filter: &tg.ChannelParticipantsAdmins{},
		Offset: 0,
		Limit:  100,
	})
	if err != nil {
		return 0, fmt.Errorf("failed to get participants: %w", err)
	}

	switch data := res.(type) {
	case *tg.ChannelsChannelParticipants:
		for _, participant := range data.Participants {
			switch p := participant.(type) {
			case *tg.ChannelParticipantCreator:
				return p.UserID, nil
			}
		}
	}

	return 0, fmt.Errorf("creator not found in admins list")
}
