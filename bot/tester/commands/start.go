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

		startText := fmt.Sprintf("Привет, %s! Я бот-тестировщик: запускаю присланных ботов на тестовом токене.\nВаш Telegram ID: %d", from.FirstName, from.ID)
		sendText(ctx, tgBot, update.Message.Chat.ID, startText)
	}
}
