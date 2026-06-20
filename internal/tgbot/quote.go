package tgbot

import (
	"fmt"
	"image"
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

	replyMsg, replyUsers, replyChatMap, err := fetchMessage(ctx, chatID, replyToMsgID)
	if err != nil || replyMsg == nil {
		return nil, err
	}

	media, err := fetchMediaGroup(ctx, chatID, replyMsg)
	if err != nil || replyMsg == nil {
		return nil, err
	}

	location, _, err := fetchStickerFromMessage(ctx, replyMsg)
	if err != nil {
		// log.Printf("failed to fetch sticker from message: %v", err)
	} else if location != nil {
		sticker, err := downloadFile(ctx, location)
		if err == nil && sticker != nil {
			media = append(media, sticker)
		} else if err != nil {
			log.Printf("failed to download sticker from message: %v", err)
		}
	}

	author := resolveAuthor(replyMsg, replyUsers)
	fwdAuthor, ok := resolveForwardAuthorFull(&replyMsg.FwdFrom, replyUsers, replyChatMap)
	if ok && fwdAuthor.FirstName != "" {
		author = fwdAuthor
	}
	result := &QuoteData{
		Author: author,
		Text:   replyMsg.Message,
		Media:  media,
	}

	innerReply, ok := replyMsg.ReplyTo.(*tg.MessageReplyHeader)
	if ok && innerReply != nil && innerReply.ReplyToMsgID != 0 {
		innerMsg, innerUsers, _, err := fetchMessage(ctx, chatID, innerReply.ReplyToMsgID)
		author := resolveAuthor(innerMsg, innerUsers)
		if err == nil && innerMsg != nil {
			text := innerMsg.Message
			if innerReply.Quote && innerReply.QuoteText != "" {
				text = innerReply.QuoteText
			}
			result.ReplyTo = &QuoteData{
				Author: author,
				Text:   text,
			}
		}
	}

	return result, nil
}

func extractQuoteDataFromStack(ctx *ext.Context, chatID int64, replyToMsgID int, replyMsg *tg.Message, replyUsers map[int64]*tg.User, replyChatMap map[int64]tg.ChatClass) (*QuoteData, error) {
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

	media, err := fetchMediaGroup(ctx, chatID, replyMsg)
	if err != nil || replyMsg == nil {
		return nil, err
	}

	location, _, err := fetchStickerFromMessage(ctx, replyMsg)
	if err != nil {
		// log.Printf("failed to fetch sticker from message: %v", err)
	} else if location != nil {
		sticker, err := downloadFile(ctx, location)
		if err == nil && sticker != nil {
			media = append(media, sticker)
		} else if err != nil {
			log.Printf("failed to download sticker from message: %v", err)
		}
	}

	author := resolveAuthor(replyMsg, replyUsers)
	fwdAuthor, ok := resolveForwardAuthorFull(&replyMsg.FwdFrom, replyUsers, replyChatMap)
	if ok && fwdAuthor.FirstName != "" {
		author = fwdAuthor
	}

	result := &QuoteData{
		Author: author,
		Text:   replyMsg.Message,
		Media:  media,
	}

	innerReply, ok := replyMsg.ReplyTo.(*tg.MessageReplyHeader)
	if ok && innerReply != nil && innerReply.ReplyToMsgID != 0 {
		innerMsg, innerUsers, _, err := fetchMessage(ctx, chatID, innerReply.ReplyToMsgID)
		author := resolveAuthor(innerMsg, innerUsers)
		if err == nil && innerMsg != nil {
			text := innerMsg.Message
			if innerReply.Quote && innerReply.QuoteText != "" {
				text = innerReply.QuoteText
			}
			result.ReplyTo = &QuoteData{
				Author: author,
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
	userID := update.EffectiveUser().ID
	userFirstName := update.EffectiveUser().FirstName
	userLastName := update.EffectiveUser().LastName

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

		authorID := quoteData.Author.ID
		authorName := quoteData.Author.FirstName
		if quoteData.Author.ID == 0 || quoteData.Author.FirstName == "" {
			authorID = userID
			authorName = userFirstName
			if userLastName != "" {
				authorName = userFirstName + " " + userLastName
			}
		}

		avatar, err := fetchAvatar(ctx, authorID)
		if err != nil {
			log.Printf("fetchAvatar: %v", err)
		}

		messages := []render.ChatMessage{
			{
				AuthorID:    authorID,
				AuthorName:  authorName,
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

		buffs, err := imageToWebP(sticker.Image())
		if err != nil {
			log.Printf("imageToWebP error: %v", err)
			return err
		}
		docClass, err := uploadMedia(ctx, chatID, buffs, "image/webp")
		if err != nil {
			log.Printf("uploadMedia error: %v", err)
			return err
		}
		doc, ok := docClass.(*tg.Document)
		if !ok {
			return fmt.Errorf("unexpected doc type: %T", docClass)
		}

		err = sendStickerToChat(ctx, chatID, doc)
		if err != nil {
			log.Printf("sendStickerToChat error: %v", err)
			return err
		}

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

		buffs, err := imageToWebP(sticker.Image())
		if err != nil {
			log.Printf("imageToWebP error: %v", err)
			return err
		}
		docClass, err := uploadMedia(ctx, chatID, buffs, "image/webp")
		if err != nil {
			log.Printf("uploadMedia error: %v", err)
			return err
		}
		doc, ok := docClass.(*tg.Document)
		if !ok {
			return fmt.Errorf("unexpected doc type: %T", docClass)
		}

		err = sendStickerToChat(ctx, chatID, doc)
		if err != nil {
			log.Printf("sendStickerToChat error: %v", err)
			return err
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
	groups, users, chatMap, err := getMessageRange(ctx, chatID, replyToMsgID, number)
	if err != nil {
		return nil, fmt.Errorf("getMessageRange: %w", err)
	}
	if len(groups) == 0 {
		return nil, fmt.Errorf("no messages found")
	}

	var chatMessages []render.ChatMessage
	avatarCache := make(map[int64]image.Image)

	for i, group := range groups {
		mainMsg := group.Messages[0]

		quoteData, err := extractQuoteDataFromStack(ctx, chatID, mainMsg.ID, mainMsg, users, chatMap)
		if err != nil {
			return nil, fmt.Errorf("extractQuoteData: %w", err)
		}
		if quoteData == nil {
			return nil, fmt.Errorf("extractQuoteData: %w", err)
		}

		if isMediaOnly(quoteData) {
			chatMessages = append(chatMessages, render.ChatMessage{
				Standalone: true,
				Media:      quoteData.Media,
			})
			continue
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
		if replyHeader.Quote && replyHeader.QuoteText != "" && i == 0 {
			text = replyHeader.QuoteText
		}

		// var media []image.Image
		// for _, msg := range group.Messages {
		// 	imgs, err := fetchMedia(ctx, msg)
		// 	if err != nil {
		// 		log.Printf("fetchMedia for msg %d: %v", msg.ID, err)
		// 		continue
		// 	}
		// 	media = append(media, imgs...)
		// }

		authorID := quoteData.Author.ID
		avatar, cached := avatarCache[authorID]

		if !cached {
			location, err := GetAvatarLocationFromPeer(users, chatMap, ctx, authorID)

			if err == nil && location != nil {
				avatar, err = downloadFile(ctx, location)
				if err != nil {
					log.Printf("avatar download failed for user %d: %v", authorID, err)
					avatar = nil
				}
			} else {
				avatar, err = fetchAvatar(ctx, authorID)
				if err != nil {
					log.Printf("fetchAvatar error: %v", err)
				}
			}

			avatarCache[authorID] = avatar
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

func isMediaOnly(quoteData *QuoteData) bool {
	return len(quoteData.Media) > 0 && strings.TrimSpace(quoteData.Text) == "" && quoteData.ReplyTo == nil
}
