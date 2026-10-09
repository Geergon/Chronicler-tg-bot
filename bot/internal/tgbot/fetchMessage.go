package tgbot

import (
	"fmt"
	"image/color"
	"log"
	"sort"

	"github.com/Geergon/Chronicler-tg-bot/internal/render"
	"github.com/celestix/gotgproto/ext"
	"github.com/gotd/td/tg"
)

type MessageGroup struct {
	Messages  []*tg.Message
	GroupedID int64
}

func fetchMessage(ctx *ext.Context, chatID int64, msgID int) (*tg.Message, map[int64]*tg.User, map[int64]tg.ChatClass, error) {
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
		return nil, nil, nil, fmt.Errorf("unsupported peer type: %T", inputPeer)
	}

	if err != nil {
		log.Printf("failed to get messages: %v", err)
		return nil, nil, nil, err
	}

	var messages []tg.MessageClass
	var users []tg.UserClass
	var chats []tg.ChatClass

	switch r := msgClass.(type) {
	case *tg.MessagesChannelMessages:
		messages = r.Messages
		users = r.Users
		chats = r.Chats
	case *tg.MessagesMessages:
		messages = r.Messages
		users = r.Users
		chats = r.Chats
	case *tg.MessagesMessagesSlice:
		messages = r.Messages
		users = r.Users
		chats = r.Chats
	default:
		return nil, nil, nil, fmt.Errorf("unknown messages response type: %T", msgClass)
	}

	if len(messages) == 0 {
		return nil, nil, nil, nil
	}

	msg, ok := messages[0].(*tg.Message)
	if !ok {
		return nil, nil, nil, nil
	}

	userMap := make(map[int64]*tg.User)
	for _, u := range users {
		if user, ok := u.(*tg.User); ok {
			userMap[user.ID] = user
		}
	}

	chatMap := make(map[int64]tg.ChatClass)
	for _, c := range chats {
		switch ch := c.(type) {
		case *tg.Chat:
			chatMap[ch.ID] = ch
		case *tg.Channel:
			chatMap[ch.ID] = ch
		}
	}
	return msg, userMap, chatMap, nil
}

func getMessageRange(ctx *ext.Context, chatID int64, startMsgID, count int) ([]MessageGroup, map[int64]*tg.User, map[int64]tg.ChatClass, error) {
	if count <= 0 || count > 6 {
		_, err := ctx.SendMessage(chatID, &tg.MessagesSendMessageRequest{
			Message: "Вказана завелика кількість повідомлень для збереження. Максимальний ліміт це 6",
		})
		if err != nil {
			log.Println("failed to send message: ", err)
		}

		return nil, nil, nil, fmt.Errorf("invalid count: %d", count)
	}

	fetchCount := count*10 + 5
	ids := make([]tg.InputMessageClass, fetchCount)
	for i := range ids {
		ids[i] = &tg.InputMessageID{ID: startMsgID + i}
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
	default:
		msgClass, err = ctx.Raw.MessagesGetMessages(ctx, ids)
	}
	if err != nil {
		return nil, nil, nil, err
	}

	var rawMessages []tg.MessageClass
	var users []tg.UserClass
	var chats []tg.ChatClass

	switch r := msgClass.(type) {
	case *tg.MessagesChannelMessages:
		rawMessages = r.Messages
		users = r.Users
		chats = r.Chats
	case *tg.MessagesMessages:
		rawMessages = r.Messages
		users = r.Users
		chats = r.Chats
	case *tg.MessagesMessagesSlice:
		rawMessages = r.Messages
		users = r.Users
		chats = r.Chats
	}

	userMap := make(map[int64]*tg.User)
	for _, u := range users {
		if user, ok := u.(*tg.User); ok {
			userMap[user.ID] = user
		}
	}

	chatMap := make(map[int64]tg.ChatClass)
	for _, c := range chats {
		switch ch := c.(type) {
		case *tg.Chat:
			chatMap[ch.ID] = ch
		case *tg.Channel:
			chatMap[ch.ID] = ch
		}
	}

	var validMsgs []*tg.Message
	for _, m := range rawMessages {
		msg, ok := m.(*tg.Message)
		if !ok {
			continue
		}
		if msg.Message == "" && msg.Media == nil {
			continue
		}
		validMsgs = append(validMsgs, msg)
	}
	sort.Slice(validMsgs, func(i, j int) bool {
		return validMsgs[i].ID < validMsgs[j].ID
	})

	var groups []MessageGroup
	seenGroupIDs := make(map[int64]int)

	for _, msg := range validMsgs {
		if msg.GroupedID != 0 {
			if idx, exists := seenGroupIDs[msg.GroupedID]; exists {
				groups[idx].Messages = append(groups[idx].Messages, msg)
				continue
			}
			seenGroupIDs[msg.GroupedID] = len(groups)
			groups = append(groups, MessageGroup{
				Messages:  []*tg.Message{msg},
				GroupedID: msg.GroupedID,
			})
		} else {
			groups = append(groups, MessageGroup{
				Messages:  []*tg.Message{msg},
				GroupedID: 0,
			})
		}

		if len(groups) == count {
			break
		}
	}

	return groups, userMap, chatMap, nil
}

func resolveForwardAuthorFull(
	fwd *tg.MessageFwdHeader,
	userMap map[int64]*tg.User,
	chatMap map[int64]tg.ChatClass,
) (MessageAuthor, bool) {
	if fwd == nil {
		return MessageAuthor{}, false
	}

	var name string
	var id int64
	var fwdAuthor MessageAuthor

	if fwd.FromName != "" {
		return MessageAuthor{ID: 1, FirstName: fwd.FromName}, true
	}

	switch peer := fwd.FromID.(type) {
	case *tg.PeerUser:
		fwdAuthor = MessageAuthor{ID: peer.UserID}
		id = peer.UserID
		if u, ok := userMap[peer.UserID]; ok {
			name = u.FirstName
			if u.LastName != "" {
				name += " " + u.LastName
			}

			fwdAuthor.FirstName = name
			fwdAuthor.ID = id
		}

	case *tg.PeerChannel:
		fwdAuthor = MessageAuthor{ID: peer.ChannelID}
		id = peer.ChannelID
		if chat, ok := chatMap[peer.ChannelID]; ok {
			name = getChatTitle(chat)
		}
		fwdAuthor.FirstName = name
		fwdAuthor.ID = id
	case *tg.PeerChat:
		fwdAuthor = MessageAuthor{ID: peer.ChatID}

		if chat, ok := chatMap[peer.ChatID]; ok {
			fwdAuthor.FirstName = getChatTitle(chat)
		}
	}

	return fwdAuthor, true
}

func GetCreatorID(ctx *ext.Context, chatID int64) (int64, error) {
	inputPeer := ctx.PeerStorage.GetInputPeerById(chatID)
	if inputPeer == nil {
		return 0, fmt.Errorf("cannot find peer in storage")
	}
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

func getChatTitle(chat tg.ChatClass) string {
	switch c := chat.(type) {
	case *tg.Chat:
		return c.Title

	case *tg.Channel:
		return c.Title
	}

	return ""
}

func parseEntities(text string, entities []tg.MessageEntityClass, defaultColor color.Color) []render.TextSegment {
	if len(entities) == 0 || text == "" {
		return []render.TextSegment{{Text: text, Color: defaultColor}}
	}

	runes := []rune(text)
	runeLen := len(runes)

	type runeStyle struct {
		Bold          bool
		Italic        bool
		Mono          bool
		Underline     bool
		Strikethrough bool
		Spoiler       bool
		Mention       bool
	}

	styles := make([]runeStyle, runeLen)

	for _, entity := range entities {
		var offset, length int
		var applyFn func(*runeStyle)

		switch e := entity.(type) {
		case *tg.MessageEntityBold:
			offset, length = e.Offset, e.Length
			applyFn = func(s *runeStyle) { s.Bold = true }
		case *tg.MessageEntityItalic:
			offset, length = e.Offset, e.Length
			applyFn = func(s *runeStyle) { s.Italic = true }
		case *tg.MessageEntityCode:
			offset, length = e.Offset, e.Length
			applyFn = func(s *runeStyle) { s.Mono = true }
		case *tg.MessageEntityPre:
			offset, length = e.Offset, e.Length
			applyFn = func(s *runeStyle) { s.Mono = true }
		case *tg.MessageEntityUnderline:
			offset, length = e.Offset, e.Length
			applyFn = func(s *runeStyle) { s.Underline = true }
		case *tg.MessageEntityStrike:
			offset, length = e.Offset, e.Length
			applyFn = func(s *runeStyle) { s.Strikethrough = true }
		case *tg.MessageEntitySpoiler:
			offset, length = e.Offset, e.Length
			applyFn = func(s *runeStyle) { s.Spoiler = true }
		case *tg.MessageEntityMention, *tg.MessageEntityTextURL, *tg.MessageEntityURL:
			switch em := entity.(type) {
			case *tg.MessageEntityMention:
				offset, length = em.Offset, em.Length
			case *tg.MessageEntityTextURL:
				offset, length = em.Offset, em.Length
			case *tg.MessageEntityURL:
				offset, length = em.Offset, em.Length
			}
			applyFn = func(s *runeStyle) { s.Mention = true }
		default:
			continue
		}

		utf16Start := offset
		utf16End := offset + length
		runeIdx := 0
		utf16Idx := 0

		startRune := -1
		endRune := runeLen

		for runeIdx < runeLen {
			if utf16Idx == utf16Start {
				startRune = runeIdx
			}
			if utf16Idx == utf16End {
				endRune = runeIdx
				break
			}
			r := runes[runeIdx]
			if r >= 0x10000 {
				utf16Idx += 2 // surrogate pair
			} else {
				utf16Idx++
			}
			runeIdx++
		}
		if startRune == -1 {
			continue
		}

		for i := startRune; i < endRune && i < runeLen; i++ {
			applyFn(&styles[i])
		}
	}

	var segments []render.TextSegment
	if runeLen == 0 {
		return segments
	}

	start := 0
	for i := 1; i <= runeLen; i++ {
		if i == runeLen || styles[i] != styles[start] {
			s := styles[start]
			seg := render.TextSegment{
				Text:          string(runes[start:i]),
				Bold:          s.Bold,
				Italic:        s.Italic,
				Mono:          s.Mono,
				Underline:     s.Underline,
				Strikethrough: s.Strikethrough,
				Spoiler:       s.Spoiler,
			}

			switch {
			case s.Mono:
				seg.Color = color.RGBA{88, 135, 167, 255} // #5887a7
			case s.Mention:
				seg.Color = color.RGBA{106, 183, 236, 255} // #6ab7ec
			case s.Spoiler:
				r, g, b, _ := defaultColor.RGBA()
				seg.Color = color.RGBA{
					R: uint8(r >> 8),
					G: uint8(g >> 8),
					B: uint8(b >> 8),
					A: 38, // ~15% opacity
				}
			default:
				seg.Color = defaultColor
			}

			segments = append(segments, seg)
			start = i
		}
	}

	return segments
}
