package tgbot

import (
	"fmt"
	"image/color"
	"log"

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
	inputPeer := ctx.PeerStorage.GetInputPeerById(chatID)
	ch, ok := inputPeer.(*tg.InputPeerChannel)
	if !ok {
		return nil, fmt.Errorf("unsupported peer type")
	}

	fetch := func(msgID int) (*tg.Message, map[int64]*tg.User, error) {
		result, err := ctx.Raw.ChannelsGetMessages(ctx, &tg.ChannelsGetMessagesRequest{
			Channel: &tg.InputChannel{
				ChannelID:  ch.ChannelID,
				AccessHash: ch.AccessHash,
			},
			ID: []tg.InputMessageClass{&tg.InputMessageID{ID: msgID}},
		})
		if err != nil {
			log.Printf("failed to get messages: %v", err)
			return nil, nil, err
		}
		msgs, ok := result.(*tg.MessagesChannelMessages)
		if !ok || len(msgs.Messages) == 0 {
			return nil, nil, nil
		}
		msg, ok := msgs.Messages[0].(*tg.Message)
		if !ok {
			return nil, nil, nil
		}

		userMap := make(map[int64]*tg.User)
		for _, u := range msgs.Users {
			if user, ok := u.(*tg.User); ok {
				userMap[user.ID] = user
			}
		}
		return msg, userMap, nil
	}

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

	replyMsg, replyUsers, err := fetch(replyToMsgID)
	if err != nil || replyMsg == nil {
		return nil, err
	}

	result := &QuoteData{
		Author: resolveAuthor(replyMsg, replyUsers),
		Text:   replyMsg.Message,
	}

	innerReply, ok := replyMsg.ReplyTo.(*tg.MessageReplyHeader)
	if ok && innerReply != nil && innerReply.ReplyToMsgID != 0 {
		innerMsg, innerUsers, err := fetch(innerReply.ReplyToMsgID)
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
}
