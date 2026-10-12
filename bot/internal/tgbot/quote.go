package tgbot

import (
	"fmt"
	"image/color"
	"log"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/Geergon/Chronicler-tg-bot/internal/renderclient"
	"github.com/celestix/gotgproto/ext"
	"github.com/gotd/td/tg"
)

type MessageAuthor struct {
	ID            int64
	FirstName     string
	LastName      string
	Username      string
	IsBot         bool
	Rank          string
	Forwarded     bool
	StatusEmojiID int64 // 0 = no emoji status
}

type QuoteData struct {
	Author   MessageAuthor
	Text     string
	Media    []renderclient.MediaItem
	ReplyTo  *QuoteData
	ChatMap  map[int64]tg.ChatClass
	UserMap  map[int64]*tg.User
	replyMsg *tg.Message
	Entities []tg.MessageEntityClass
}

var replyTextColor = color.RGBA{209, 209, 209, 255}

func extractQuoteData(ctx *ext.Context, chatID int64, replyToMsgID int) (*QuoteData, error) {
	resolveAuthor := func(msg *tg.Message, userMap map[int64]*tg.User) MessageAuthor {
		if msg == nil {
			return MessageAuthor{}
		}

		if msg.FromID == nil {
			return MessageAuthor{
				ID:        0,
				FirstName: "",
			}
		}

		peer, ok := msg.FromID.(*tg.PeerUser)
		if !ok {
			return MessageAuthor{}
		}
		a := MessageAuthor{ID: peer.UserID}
		if rank, ok := msg.GetFromRank(); ok {
			a.Rank = rank
		}
		if u, ok := userMap[peer.UserID]; ok {
			a.FirstName = u.FirstName
			a.LastName = u.LastName
			a.Username = u.Username
			a.IsBot = u.Bot
			a.StatusEmojiID = statusEmojiID(u)
		}
		return a
	}

	msg, users, chats, err := fetchMessage(ctx, chatID, replyToMsgID)
	if err != nil || msg == nil {
		return nil, err
	}

	media, err := fetchMediaGroup(ctx, chatID, msg)
	if err != nil {
		log.Printf("fetchMediaGroup: %v", err)
	}

	location, _, err := fetchStickerFromMessage(ctx, msg)
	if err != nil {
		// log.Printf("failed to fetch sticker from message: %v", err)
	} else if location != nil {
		sticker, err := downloadFile(ctx, location)
		if err == nil && sticker != nil {
			media = append(media, renderclient.MediaItem{Data: sticker, Spoiler: false})
		} else if err != nil {
			log.Printf("failed to download sticker from message: %v", err)
		}
	}

	author := resolveAuthor(msg, users)
	fwdAuthor, ok := resolveForwardAuthorFull(&msg.FwdFrom, users, chats)
	if ok && fwdAuthor.FirstName != "" {
		author = fwdAuthor
		author.Forwarded = true
	}

	result := &QuoteData{
		Author:   author,
		Text:     msg.Message,
		Media:    media,
		UserMap:  users,
		ChatMap:  chats,
		replyMsg: msg,
	}

	innerReply, ok := msg.ReplyTo.(*tg.MessageReplyHeader)
	if ok && innerReply != nil && innerReply.ReplyToMsgID != 0 {
		innerMsg, innerUsers, _, err := fetchMessage(ctx, chatID, innerReply.ReplyToMsgID)
		author := resolveAuthor(innerMsg, innerUsers)
		if err == nil && innerMsg != nil {
			text := innerMsg.Message
			entities := innerMsg.Entities
			if innerReply.Quote && innerReply.QuoteText != "" {
				text = innerReply.QuoteText
				entities = innerReply.QuoteEntities
			}
			result.ReplyTo = &QuoteData{
				Author:   author,
				Text:     text,
				Entities: entities,
			}
		}
	}

	return result, nil
}

func extractQuoteDataFromStack(ctx *ext.Context, chatID int64, replyToMsgID int, replyMsg *tg.Message, replyUsers map[int64]*tg.User, replyChatMap map[int64]tg.ChatClass) (*QuoteData, error) {
	resolveAuthor := func(msg *tg.Message, userMap map[int64]*tg.User) MessageAuthor {
		if msg == nil {
			log.Println("resolveAuthor: msg is nil")
			return MessageAuthor{}
		}
		if msg.FromID == nil {
			return MessageAuthor{
				ID:        0,
				FirstName: "",
			}
		}
		peer, ok := msg.FromID.(*tg.PeerUser)
		if !ok {
			return MessageAuthor{}
		}
		a := MessageAuthor{ID: peer.UserID}
		if rank, ok := msg.GetFromRank(); ok {
			a.Rank = rank
		}
		if u, ok := userMap[peer.UserID]; ok {
			a.FirstName = u.FirstName
			a.LastName = u.LastName
			a.Username = u.Username
			a.IsBot = u.Bot
			a.StatusEmojiID = statusEmojiID(u)
		}
		return a
	}

	media, err := fetchMediaGroup(ctx, chatID, replyMsg)
	if err != nil {
		log.Printf("fetchMediaGroup: %v", err)
	}

	location, _, err := fetchStickerFromMessage(ctx, replyMsg)
	if err != nil {
		// log.Printf("failed to fetch sticker from message: %v", err)
	} else if location != nil {
		sticker, err := downloadFile(ctx, location)
		if err == nil && sticker != nil {
			media = append(media, renderclient.MediaItem{Data: sticker, Spoiler: false})
		} else if err != nil {
			log.Printf("failed to download sticker from message: %v", err)
		}
	}

	author := resolveAuthor(replyMsg, replyUsers)
	fwdAuthor, ok := resolveForwardAuthorFull(&replyMsg.FwdFrom, replyUsers, replyChatMap)
	if ok && fwdAuthor.FirstName != "" {
		author = fwdAuthor
		author.Forwarded = true
	}

	result := &QuoteData{
		Author:   author,
		Text:     replyMsg.Message,
		Media:    media,
		replyMsg: replyMsg,
	}

	innerReply, ok := replyMsg.ReplyTo.(*tg.MessageReplyHeader)
	if ok && innerReply != nil && innerReply.ReplyToMsgID != 0 {
		innerMsg, innerUsers, _, err := fetchMessage(ctx, chatID, innerReply.ReplyToMsgID)
		author := resolveAuthor(innerMsg, innerUsers)
		if err == nil && innerMsg != nil {
			text := innerMsg.Message
			entities := innerMsg.Entities
			if innerReply.Quote && innerReply.QuoteText != "" {
				text = innerReply.QuoteText
				entities = innerReply.QuoteEntities
			}
			result.ReplyTo = &QuoteData{
				Author:   author,
				Text:     text,
				Entities: entities,
			}
		}
	}

	return result, nil
}

func HandleQuote(ctx *ext.Context, update *ext.Update, replyEnable bool) error {
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

	lg := func(format string, a ...any) {
		log.Printf("[chat=%d msg=%d] "+format, append([]any{chatID, msg.ID}, a...)...)
	}
	args := strings.Fields(update.EffectiveMessage.Text)
	lg("args=%q reply=%v", args, replyEnable)

	if len(args) == 1 {

		quoteData, err := extractQuoteData(ctx, chatID, replyHeader.ReplyToMsgID)
		if err != nil {
			return fmt.Errorf("extractQuoteData: %w", err)
		}
		if quoteData == nil {
			return fmt.Errorf("extractQuoteData nil")
		}
		lg("quote data: author=%d forwarded=%v text=%d runes media=%d has_reply=%v",
			quoteData.Author.ID, quoteData.Author.Forwarded,
			utf8.RuneCountInString(quoteData.Text), len(quoteData.Media), quoteData.ReplyTo != nil)

		var replyInfo *renderclient.Reply
		if replyEnable {
			if quoteData.ReplyTo != nil {
				authorName := quoteData.ReplyTo.Author.FirstName
				if quoteData.ReplyTo.Author.LastName != "" {
					authorName = quoteData.ReplyTo.Author.FirstName + " " + quoteData.ReplyTo.Author.LastName
				}
				entities := parseEntities(quoteData.ReplyTo.Text, quoteData.ReplyTo.Entities, replyTextColor)
				replyInfo = &renderclient.Reply{
					AuthorID:   quoteData.ReplyTo.Author.ID,
					AuthorName: authorName,
					Text:       quoteData.ReplyTo.Text,
					Segments:   entities,
				}
				attachCustomEmoji(ctx, entities)
			}
		}

		var tag *renderclient.Tag
		if !quoteData.Author.Forwarded && quoteData.Author.ID != 0 {
			tag = resolveTag(getChatRoles(ctx, chatID), quoteData.Author.ID, quoteData.Author.Rank)
		}

		// quoted text instead of actual text if user quoted part of the text in the /q command
		text := quoteData.Text
		var entities []renderclient.Segment
		if replyHeader.Quote && replyHeader.QuoteText != "" {
			text = replyHeader.QuoteText
			entities = parseEntities(text, replyHeader.QuoteEntities, color.RGBA{255, 255, 255, 255})
		} else {
			entities = parseEntities(text, quoteData.replyMsg.Entities, color.RGBA{255, 255, 255, 255})
		}
		attachCustomEmoji(ctx, entities)
		lg("segments=%d", len(entities))

		authorID := quoteData.Author.ID
		authorName := quoteData.Author.FirstName
		if quoteData.Author.LastName != "" {
			authorName = quoteData.Author.FirstName + " " + quoteData.Author.LastName
		}
		if quoteData.Author.ID == 0 || quoteData.Author.FirstName == "" {
			authorID = userID
			authorName = userFirstName
			if userLastName != "" {
				authorName = userFirstName + " " + userLastName
			}
		}

		var nameEmoji []byte
		if id := quoteData.Author.StatusEmojiID; id != 0 {
			nameEmoji = loadCustomEmoji(ctx, []int64{id})[id]
		}

		avatar := getAvatar(ctx, authorID, quoteData.Author.Username, quoteData.UserMap, quoteData.ChatMap)
		lg("avatar: %d bytes", len(avatar))

		messages := []renderclient.Message{
			{
				AuthorID:    authorID,
				AuthorName:  authorName,
				Tag:         tag,
				Reply:       replyInfo,
				Avatar:      avatar,
				BubbleColor: color.RGBA{45, 40, 60, 255},
				Media:       quoteData.Media,
				Segments:    entities,
				NameEmoji:   nameEmoji,
			},
		}

		return renderAndSend(ctx, chatID, messages)

	} else if len(args) == 2 {
		numberString := args[1]
		number, err := strconv.Atoi(numberString)
		if err != nil {
			log.Printf("failed to format string to int: %v", err)
			return err
		}

		messages, err := handleMessageStack(ctx, chatID, replyHeader.ReplyToMsgID, number, replyHeader, replyEnable)
		if err != nil {
			log.Printf("failed to handle messages stack: %v", err)
			return err
		}

		return renderAndSend(ctx, chatID, messages)

	} else {
		log.Println("too many arguments in /q command")
		return nil
	}
}

func handleMessageStack(ctx *ext.Context, chatID int64, replyToMsgID int, number int, replyHeader *tg.MessageReplyHeader, replyEnable bool) ([]renderclient.Message, error) {
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

	var chatMessages []renderclient.Message

	roles := getChatRoles(ctx, chatID)

	for _, group := range groups {
		var avatar []byte
		mainMsg := group.Messages[0]

		quoteData, err := extractQuoteDataFromStack(ctx, chatID, mainMsg.ID, mainMsg, users, chatMap)
		if err != nil {
			return nil, fmt.Errorf("extractQuoteData: %w", err)
		}
		if quoteData == nil {
			return nil, fmt.Errorf("extractQuoteData error: quoteData is nil")
		}

		if isMediaOnly(quoteData) {
			chatMessages = append(chatMessages, renderclient.Message{
				Standalone: true,
				Media:      quoteData.Media,
			})
			continue
		}

		var replyInfo *renderclient.Reply
		if replyEnable {
			if quoteData.ReplyTo != nil {
				authorName := quoteData.ReplyTo.Author.FirstName
				if quoteData.ReplyTo.Author.LastName != "" {
					authorName = quoteData.ReplyTo.Author.FirstName + " " + quoteData.ReplyTo.Author.LastName
				}
				entities := parseEntities(quoteData.ReplyTo.Text, quoteData.ReplyTo.Entities, replyTextColor)
				replyInfo = &renderclient.Reply{
					AuthorID:   quoteData.ReplyTo.Author.ID,
					AuthorName: authorName,
					Text:       quoteData.ReplyTo.Text,
					Segments:   entities,
				}
				attachCustomEmoji(ctx, entities)
			}
		}

		// quoted text instead of actual text if user quoted part of the text in the /q command
		text := quoteData.Text
		var entities []renderclient.Segment
		if replyHeader.Quote && replyHeader.QuoteText != "" {
			text = replyHeader.QuoteText
			entities = parseEntities(text, replyHeader.QuoteEntities, color.RGBA{255, 255, 255, 255})
		} else {
			entities = parseEntities(text, quoteData.replyMsg.Entities, color.RGBA{255, 255, 255, 255})
		}
		attachCustomEmoji(ctx, entities)

		var tag *renderclient.Tag
		if !quoteData.Author.Forwarded {
			tag = resolveTag(roles, quoteData.Author.ID, quoteData.Author.Rank)
		}

		authorID := quoteData.Author.ID
		authorName := quoteData.Author.FirstName
		if quoteData.Author.LastName != "" {
			authorName = quoteData.Author.FirstName + " " + quoteData.Author.LastName
		}

		var nameEmoji []byte
		if id := quoteData.Author.StatusEmojiID; id != 0 && !quoteData.Author.Forwarded {
			nameEmoji = loadCustomEmoji(ctx, []int64{id})[id]
		}

		avatar = getAvatar(ctx, authorID, quoteData.Author.Username, users, chatMap)

		chatMessages = append(chatMessages, renderclient.Message{
			AuthorID:    authorID,
			AuthorName:  authorName,
			Avatar:      avatar,
			Tag:         tag,
			Reply:       replyInfo,
			BubbleColor: color.RGBA{45, 40, 60, 255},
			Media:       quoteData.Media,
			Segments:    entities,
			NameEmoji:   nameEmoji,
		})
	}

	return chatMessages, nil
}

func isMediaOnly(quoteData *QuoteData) bool {
	return len(quoteData.Media) > 0 && strings.TrimSpace(quoteData.Text) == "" && quoteData.ReplyTo == nil
}
