package tgbot

import (
	"fmt"
	"image"
	"image/color"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/Geergon/Chronicler-tg-bot/internal/render"
	"github.com/HugoSmits86/nativewebp"
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
	Media   []image.Image
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

	media, err := fetchMedia(ctx, replyMsg)
	if err != nil || replyMsg == nil {
		return nil, err
	}

	result := &QuoteData{
		Author: resolveAuthor(replyMsg, replyUsers),
		Text:   replyMsg.Message,
		Media:  media,
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

	media, err := fetchMedia(ctx, replyMsg)
	if err != nil || replyMsg == nil {
		return nil, err
	}

	result := &QuoteData{
		Author: resolveAuthor(replyMsg, replyUsers),
		Text:   replyMsg.Message,
		Media:  media,
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

		avatar, err := fetchUserAvatar(ctx, quoteData.Author.ID)
		if err != nil {
			log.Printf("fetchUserAvatar: %v", err)
		}

		messages := []render.ChatMessage{
			{
				AuthorID:    quoteData.Author.ID,
				AuthorName:  quoteData.Author.FirstName,
				Reply:       replyInfo,
				AvatarImg:   avatar,
				BubbleColor: color.RGBA{45, 40, 60, 255},
				Media:       quoteData.Media,
				Segments: []render.TextSegment{
					{Text: text, Color: color.RGBA{255, 255, 255, 255}},
				},
			},
		}

		sticker, err := render.BuildStickerChatStack(messages)
		if err != nil {
			return fmt.Errorf("BuildStickerChatStack: %w", err)
		}

		stickerFileName := "out.webp"
		file, err := os.Create(stickerFileName)
		if err != nil {
			log.Fatalf("Error creating file %s: %v", stickerFileName, err)
		}
		defer file.Close()

		err = nativewebp.Encode(file, sticker.Image(), nil)
		if err != nil {
			log.Fatalf("Error encoding image to WebP: %v", err)
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

		avatar, err := fetchUserAvatar(ctx, quoteData.Author.ID)
		if err != nil {
			log.Printf("fetchUserAvatar: %v", err)
		}

		chatMessages = append(chatMessages, render.ChatMessage{
			AuthorID:    quoteData.Author.ID,
			AuthorName:  quoteData.Author.FirstName,
			AvatarImg:   avatar,
			Reply:       replyInfo,
			BubbleColor: color.RGBA{45, 40, 60, 255},
			Media:       quoteData.Media,
			Segments: []render.TextSegment{
				{Text: text, Color: color.RGBA{255, 255, 255, 255}},
			},
		})
	}

	return chatMessages, nil
}
