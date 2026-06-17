package tgbot

import (
	"fmt"
	"image/color"
	"log"
	"strconv"
	"strings"

	"github.com/Geergon/Chronicler-tg-bot/internal/render"
	"github.com/celestix/gotgproto/ext"
	"github.com/gotd/td/tg"
)

type MessageAuthor struct {
	ID        int64
	FirstName string
	Username  string
	IsBot     bool
}

type QuoteData struct {
	Author  MessageAuthor
	Text    string
	ReplyTo *QuoteData
}

func extractQuoteData(ctx *ext.Context, chatID int64, replyToMsgID int) (*QuoteData, error) {
	resolveAuthor := func(msg *tg.Message, userMap map[int64]*tg.User) MessageAuthor {
		peer, ok := msg.FromID.(*tg.PeerUser)
		if !ok {
			return MessageAuthor{}
		}
		a := MessageAuthor{ID: peer.UserID}
		if u, ok := userMap[peer.UserID]; ok {
			a.FirstName = u.FirstName
			a.Username = u.Username
			a.IsBot = u.Bot
		}
		return a
	}

	replyMsg, replyUsers, err := fetchMessage(ctx, chatID, replyToMsgID)
	if err != nil || replyMsg == nil {
		return nil, err
	}

	result := &QuoteData{
		Author: resolveAuthor(replyMsg, replyUsers),
		Text:   replyMsg.Message,
	}

	innerReply, ok := replyMsg.ReplyTo.(*tg.MessageReplyHeader)
	if ok && innerReply != nil && innerReply.ReplyToMsgID != 0 {
		innerMsg, innerUsers, err := fetchMessage(ctx, chatID, innerReply.ReplyToMsgID)
		if err == nil && innerMsg != nil {
			text := innerMsg.Message
			if innerReply.Quote && innerReply.QuoteText != "" {
				text = innerReply.QuoteText
			}
			result.ReplyTo = &QuoteData{
				Author: resolveAuthor(innerMsg, innerUsers),
				Text:   text,
			}
		}
	}

	return result, nil
}

func extractQuoteDataFromStack(ctx *ext.Context, chatID int64, replyToMsgID int, replyMsg *tg.Message, replyUsers map[int64]*tg.User) (*QuoteData, error) {
	resolveAuthor := func(msg *tg.Message, userMap map[int64]*tg.User) MessageAuthor {
		peer, ok := msg.FromID.(*tg.PeerUser)
		if !ok {
			return MessageAuthor{}
		}
		a := MessageAuthor{ID: peer.UserID}
		if u, ok := userMap[peer.UserID]; ok {
			a.FirstName = u.FirstName
			a.Username = u.Username
			a.IsBot = u.Bot
		}
		return a
	}

	result := &QuoteData{
		Author: resolveAuthor(replyMsg, replyUsers),
		Text:   replyMsg.Message,
	}

	innerReply, ok := replyMsg.ReplyTo.(*tg.MessageReplyHeader)
	if ok && innerReply != nil && innerReply.ReplyToMsgID != 0 {
		innerMsg, innerUsers, err := fetchMessage(ctx, chatID, innerReply.ReplyToMsgID)
		if err == nil && innerMsg != nil {
			text := innerMsg.Message
			if innerReply.Quote && innerReply.QuoteText != "" {
				text = innerReply.QuoteText
			}
			result.ReplyTo = &QuoteData{
				Author: resolveAuthor(innerMsg, innerUsers),
				Text:   text,
			}
		}
	}

	return result, nil
}

func HandleQuote(ctx *ext.Context, update *ext.Update) error {
	msg := update.EffectiveMessage
	if msg == nil {
		return nil
	}

	replyHeader, ok := msg.ReplyTo.(*tg.MessageReplyHeader)
	if !ok || replyHeader == nil {
		log.Println("reply header is empty")
		return nil
	}

	chatID := update.EffectiveChat().GetID()

	args := strings.Fields(update.EffectiveMessage.Text)

	if len(args) == 1 {

		quoteData, err := extractQuoteData(ctx, chatID, replyHeader.ReplyToMsgID)
		if err != nil {
			return fmt.Errorf("extractQuoteData: %w", err)
		}
		if quoteData == nil {
			return fmt.Errorf("extractQuoteData: %w", err)
		}

		var replyInfo *render.ReplyInfo
		if quoteData.ReplyTo != nil {
			replyInfo = &render.ReplyInfo{
				AuthorID:   quoteData.ReplyTo.Author.ID,
				AuthorName: quoteData.ReplyTo.Author.FirstName,
				Text:       quoteData.ReplyTo.Text,
			}
		}

		// quoted text instead of actual text if user quoted part of the text in the /q command
		text := quoteData.Text
		if replyHeader.Quote && replyHeader.QuoteText != "" {
			text = replyHeader.QuoteText
		}

		messages := []render.ChatMessage{
			{
				AuthorID:    quoteData.Author.ID,
				AuthorName:  quoteData.Author.FirstName,
				Reply:       replyInfo,
				BubbleColor: color.RGBA{45, 40, 60, 255},
				Segments: []render.TextSegment{
					{Text: text, Color: color.RGBA{255, 255, 255, 255}},
				},
			},
		}

		sticker, err := render.BuildStickerChatStack(messages)
		if err != nil {
			return fmt.Errorf("BuildStickerChatStack: %w", err)
		}

		if err := render.SavePNG("out_chat_stack.png", sticker.Image()); err != nil {
			log.Fatal("save:", err)
		}

		// // TODO: відправити sticker як документ/стікер в чат
		// _ = sticker

		return nil
	} else if len(args) == 2 {
		numberString := args[1]
		number, err := strconv.Atoi(numberString)
		if err != nil {
			log.Printf("failed to format string to int: %v", err)
			return err
		}

		messages, err := handleMessageStack(ctx, chatID, replyHeader.ReplyToMsgID, number, replyHeader)
		if err != nil {
			log.Printf("failed to handle messages stack: %v", err)
			return err
		}

		sticker, err := render.BuildStickerChatStack(messages)
		if err != nil {
			return fmt.Errorf("BuildStickerChatStack: %w", err)
		}

		if err := render.SavePNG("out_chat_stack.png", sticker.Image()); err != nil {
			log.Fatal("save:", err)
		}

	} else {
		log.Println("too many arguments in /q command")
		return nil
	}
	return nil
}

func handleMessageStack(ctx *ext.Context, chatID int64, replyToMsgID int, number int, replyHeader *tg.MessageReplyHeader) ([]render.ChatMessage, error) {
	if number == 0 {
		return nil, fmt.Errorf("number is 0")
	}
	messagesStack, users, err := getHistory(ctx, chatID, replyToMsgID, number)
	if err != nil {
		log.Printf("failed to get messages history: %v", err)
		return nil, err
	}

	var chatMessages []render.ChatMessage
	for _, msg := range messagesStack {
		quoteData, err := extractQuoteDataFromStack(ctx, chatID, msg.ID, msg, users)
		if err != nil {
			return nil, fmt.Errorf("extractQuoteData: %w", err)
		}
		if quoteData == nil {
			return nil, fmt.Errorf("extractQuoteData: %w", err)
		}

		var replyInfo *render.ReplyInfo
		if quoteData.ReplyTo != nil {
			replyInfo = &render.ReplyInfo{
				AuthorID:   quoteData.ReplyTo.Author.ID,
				AuthorName: quoteData.ReplyTo.Author.FirstName,
				Text:       quoteData.ReplyTo.Text,
			}
		}

		// quoted text instead of actual text if user quoted part of the text in the /q command
		text := quoteData.Text
		if replyHeader.Quote && replyHeader.QuoteText != "" {
			text = replyHeader.QuoteText
		}

		chatMessages = append(chatMessages, render.ChatMessage{
			AuthorID:    quoteData.Author.ID,
			AuthorName:  quoteData.Author.FirstName,
			Reply:       replyInfo,
			BubbleColor: color.RGBA{45, 40, 60, 255},
			Segments: []render.TextSegment{
				{Text: text, Color: color.RGBA{255, 255, 255, 255}},
			},
		})
	}

	return chatMessages, nil
}

func getHistory(ctx *ext.Context, chatID int64, msgID int, limit int) ([]*tg.Message, map[int64]*tg.User, error) {
	if limit <= 0 || limit > 6 {
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
