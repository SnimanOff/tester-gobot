package commands

import (
	"context"
	"log"

	telegram "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func sendText(ctx context.Context, tgBot *telegram.Bot, chatID int64, text string) {
	_, err := tgBot.SendMessage(ctx, &telegram.SendMessageParams{
		ChatID: chatID,
		Text:   text,
	})
	if err != nil {
		log.Printf("send message to %d: %v", chatID, err)
	}
}

func sendHTML(ctx context.Context, tgBot *telegram.Bot, chatID int64, text string) {
	_, err := tgBot.SendMessage(ctx, &telegram.SendMessageParams{
		ChatID:    chatID,
		Text:      text,
		ParseMode: models.ParseModeHTML,
	})
	if err != nil {
		log.Printf("send html message to %d: %v", chatID, err)
	}
}
