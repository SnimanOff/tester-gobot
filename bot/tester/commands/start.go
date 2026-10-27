package commands

import (
	"context"
	"fmt"

	telegram "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func StartCommand() telegram.HandlerFunc {
	return func(ctx context.Context, tgBot *telegram.Bot, update *models.Update) {
		if update.Message == nil || update.Message.From == nil {
			return
		}

		from := update.Message.From

		startText := fmt.Sprintf("ID: %d", from.ID)
		sendText(ctx, tgBot, update.Message.Chat.ID, startText)
	}
}
