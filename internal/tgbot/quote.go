package tgbot

import (
	"context"
	"fmt"
	"log"

	"github.com/mymmrac/telego"
)

func GenerateQuote(ctx context.Context, update telego.Update) error {
	if update.Message == nil {
		log.Println("message is empty")
		return nil
	}

	chatID := update.Message.Chat.ID

	var userID int64
	var userFirstName string
	if update.Message.From != nil {
		userID = update.Message.From.ID
		userFirstName = update.Message.From.FirstName
	}

	text := update.Message.Text
	if update.Message.Quote != nil {
		text = update.Message.Quote.Text
	}

	var replyText string
	var replySenderID inint64
	var replySenderFirstName string
	var replySenderLastName string
	var replyMessageQuote string
	if update.Message.ReplyToMessage != nil {
		replyText := update.Message.ReplyToMessage.Text
		fmt.Printf("Reply Text: %s\n", replyText)

		if update.Message.ReplyToMessage.From != nil {
			replySenderID = update.Message.ReplyToMessage.From.ID
			replySenderFirstName = update.Message.ReplyToMessage.From.FirstName
			if update.Message.ReplyToMessage.From.LastName != "" {
				replySenderLastName = update.Message.ReplyToMessage.From.LastName
			}
		}

		if update.Message.ReplyToMessage.Quote != nil {
			replyMessageQuote = update.Message.ReplyToMessage.Quote.Text
		}

		// if update.Message.ReplyToMessage.ForwardOrigin != nil {
		// 	o := update.Message.ReplyToMessage.ForwardOrigin
		// 	o1 := update.Message.ReplyToMessage.ForwardOrigin.OriginType()
		// 	fmt.Printf("Forward Origin: %+v\n", o)
		// 	fmt.Printf("Forward Origin Type: %s\n", o1)
		// }

	} else {
		log.Println("message is not reply (ReplyToMessage is nil)")
	}

	return nil
}
