package commands

import (
	"context"

	"github.com/SnimanOff/testbot-platform/bot/children/launcher"
	telegram "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func StopChildCommand(childLauncher *launcher.Launcher, ownerTelegramID int64) telegram.HandlerFunc {
	return func(ctx context.Context, tgBot *telegram.Bot, update *models.Update) {
		if update.Message == nil || update.Message.From == nil {
			return
		}

		from := update.Message.From
		chatID := update.Message.Chat.ID

		isStopped := false
		if from.ID == ownerTelegramID {
			if _, isRunning := childLauncher.ActiveOwner(); isRunning {
				childLauncher.StopActive()
				isStopped = true
			}
		} else {
			isStopped = childLauncher.StopOwned(from.ID)
		}

		if !isStopped {
			sendText(ctx, tgBot, chatID, "Нет работающего дочернего бота, которого вы можете остановить.")
			return
		}
		sendText(ctx, tgBot, chatID, "Дочерний бот остановлен.")
	}
}
