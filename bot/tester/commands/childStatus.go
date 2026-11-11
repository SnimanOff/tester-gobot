package commands

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	databaseModels "github.com/SnimanOff/testbot-platform/database/models"
	"github.com/SnimanOff/testbot-platform/database/repositories"
	telegram "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

var childStatusTitles = map[string]string{
	databaseModels.ChildStatusChecking: "загрузка",
	databaseModels.ChildStatusBuilding: "сборка",
	databaseModels.ChildStatusRunning:  "работает",
	databaseModels.ChildStatusFailed:   "ошибка",
	databaseModels.ChildStatusStopped:  "остановлен",
}

func ChildStatusCommand(childRepository *repositories.ChildBotRepository, managedBotRepository *repositories.ManagedBotRepository) telegram.HandlerFunc {
	return func(ctx context.Context, tgBot *telegram.Bot, update *models.Update) {
		if update.Message == nil || update.Message.From == nil {
			return
		}

		chatID := update.Message.Chat.ID

		managedBots, err := managedBotRepository.List(ctx)
		if err != nil {
			log.Printf("list managed bots: %v", err)
			sendText(ctx, tgBot, chatID, "Ошибка получения статуса.")
			return
		}
		if len(managedBots) == 0 {
			sendText(ctx, tgBot, chatID, "Тестовых ботов нет. Создайте бота: /newbot")
			return
		}

		var statusText strings.Builder
		for _, managedBot := range managedBots {
			fmt.Fprintf(&statusText, "%d. @%s - ", managedBot.Number, managedBot.Username)

			childBot, err := childRepository.GetBySlot(ctx, managedBot.Number)
			if errors.Is(err, repositories.ErrorChildBotNotFound) {
				statusText.WriteString("архив не загружен\n")
				continue
			}
			if err != nil {
				log.Printf("get child bot %d: %v", managedBot.Number, err)
				sendText(ctx, tgBot, chatID, "Ошибка получения статуса.")
				return
			}

			fmt.Fprintf(&statusText, "%s, %s\n", childStatusTitles[childBot.Status], childBot.ArchiveName)
			if childBot.LastError != "" {
				fmt.Fprintf(&statusText, "Причина: %s\n", childBot.LastError)
			}
		}
		sendText(ctx, tgBot, chatID, statusText.String())
	}
}
