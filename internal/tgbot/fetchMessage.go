package tgbot

import (
	"fmt"
	"log"

	"github.com/celestix/gotgproto/ext"
	"github.com/gotd/td/tg"
)

func fetchMessage(ctx *ext.Context, chatID int64, msgID int) (*tg.Message, map[int64]*tg.User, error) {
	inputPeer := ctx.PeerStorage.GetInputPeerById(chatID)
	var msgClass tg.MessagesMessagesClass
	var err error

	switch peer := inputPeer.(type) {
	case *tg.InputPeerChannel:
		msgClass, err = ctx.Raw.ChannelsGetMessages(ctx, &tg.ChannelsGetMessagesRequest{
			Channel: &tg.InputChannel{
				ChannelID:  peer.ChannelID,
				AccessHash: peer.AccessHash,
			},
			ID: []tg.InputMessageClass{&tg.InputMessageID{ID: msgID}},
		})
	case *tg.InputPeerUser, *tg.InputPeerChat, *tg.InputPeerSelf:
		msgClass, err = ctx.Raw.MessagesGetMessages(ctx, []tg.InputMessageClass{
			&tg.InputMessageID{ID: msgID},
		})
	default:
		return nil, nil, fmt.Errorf("unsupported peer type: %T", inputPeer)
	}

	if err != nil {
		log.Printf("failed to get messages: %v", err)
		return nil, nil, err
	}

	var messages []tg.MessageClass
	var users []tg.UserClass

	switch r := msgClass.(type) {
	case *tg.MessagesChannelMessages:
		messages = r.Messages
		users = r.Users
	case *tg.MessagesMessages:
		messages = r.Messages
		users = r.Users
	case *tg.MessagesMessagesSlice:
		messages = r.Messages
		users = r.Users
	default:
		return nil, nil, fmt.Errorf("unknown messages response type: %T", msgClass)
	}

	if len(messages) == 0 {
		return nil, nil, nil
	}

	msg, ok := messages[0].(*tg.Message)
	if !ok {
		return nil, nil, nil
	}

	userMap := make(map[int64]*tg.User)
	for _, u := range users {
		if user, ok := u.(*tg.User); ok {
			userMap[user.ID] = user
		}
	}
	return msg, userMap, nil
}

func getHistory(ctx *ext.Context, chatID int64, msgID int, limit int) ([]*tg.Message, map[int64]*tg.User, error) {
	if limit <= 0 || limit > 6 {
		_, err := ctx.SendMessage(chatID, &tg.MessagesSendMessageRequest{
			Message: "Вказана завелика кількість повідомлень для збереження. Максимальний ліміт це 6",
		})
		if err != nil {
			log.Println("failed to send message: ", err)
		}

		return nil, nil, fmt.Errorf("invalid limit: %d", limit)
	}

	fetchLimit := limit + 5
	ids := make([]tg.InputMessageClass, fetchLimit)
	for i := range ids {
		ids[i] = &tg.InputMessageID{ID: msgID + i}
	}

	inputPeer := ctx.PeerStorage.GetInputPeerById(chatID)
	var msgClass tg.MessagesMessagesClass
	var err error

	switch peer := inputPeer.(type) {
	case *tg.InputPeerChannel:
		msgClass, err = ctx.Raw.ChannelsGetMessages(ctx, &tg.ChannelsGetMessagesRequest{
			Channel: &tg.InputChannel{
				ChannelID:  peer.ChannelID,
				AccessHash: peer.AccessHash,
			},
			ID: ids,
		})
	case *tg.InputPeerUser, *tg.InputPeerChat, *tg.InputPeerSelf:
		msgClass, err = ctx.Raw.MessagesGetMessages(ctx, ids)
	default:
		return nil, nil, fmt.Errorf("unsupported peer type: %T", inputPeer)
	}

	if err != nil {
		log.Printf("failed to get messages: %v", err)
		return nil, nil, err
	}
	var rawMessages []tg.MessageClass
	var users []tg.UserClass

	switch r := msgClass.(type) {
	case *tg.MessagesChannelMessages:
		rawMessages = r.Messages
		users = r.Users
	case *tg.MessagesMessages:
		rawMessages = r.Messages
		users = r.Users
	case *tg.MessagesMessagesSlice:
		rawMessages = r.Messages
		users = r.Users
	default:
		return nil, nil, fmt.Errorf("unknown messages response type: %T", msgClass)
	}

	userMap := make(map[int64]*tg.User)
	for _, u := range users {
		if user, ok := u.(*tg.User); ok {
			userMap[user.ID] = user
		}
	}

	var messages []*tg.Message
	for _, m := range rawMessages {
		msg, ok := m.(*tg.Message)
		if !ok {
			continue // MessageEmpty або MessageService
		}
		if msg.Message == "" && msg.Media == nil {
			continue // empty
		}
		messages = append(messages, msg)
		if len(messages) == limit {
			break
		}
	}

	return messages, userMap, nil
}
