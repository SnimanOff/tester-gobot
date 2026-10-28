package commands

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/SnimanOff/testbot-platform/bot/children/launcher"
	databaseModels "github.com/SnimanOff/testbot-platform/database/models"
	"github.com/SnimanOff/testbot-platform/database/repositories"
	telegram "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

var childStatusTitles = map[string]string{
	databaseModels.ChildStatusChecking: "проверка архива",
	databaseModels.ChildStatusBuilding: "сборка",
	databaseModels.ChildStatusRunning:  "работает",
	databaseModels.ChildStatusFailed:   "ошибка",
	databaseModels.ChildStatusStopped:  "остановлен",
}

func ChildStatusCommand(childRepository *repositories.ChildBotRepository, childLauncher *launcher.Launcher, childBotUsername string) telegram.HandlerFunc {
	return func(ctx context.Context, tgBot *telegram.Bot, update *models.Update) {
		if update.Message == nil || update.Message.From == nil {
			return
		}

		from := update.Message.From
		chatID := update.Message.Chat.ID

		childBot, err := childRepository.GetByOwnerID(ctx, from.ID)
		isNotFound := errors.Is(err, repositories.ErrorChildBotNotFound)
		if err != nil && !isNotFound {
			log.Printf("get child bot %d: %v", from.ID, err)
			sendText(ctx, tgBot, chatID, "Не удалось получить статус, попробуйте позже.")
			return
		}

		var statusText strings.Builder
		if isNotFound {
			statusText.WriteString("Вы еще не присылали ботов.")
		} else {
			fmt.Fprintf(&statusText, "Архив: %s\n", childBot.ArchiveName)
			fmt.Fprintf(&statusText, "Статус: %s\n", childStatusTitles[childBot.Status])
			fmt.Fprintf(&statusText, "Обновлен: %s\n", time.Unix(childBot.UpdatedAt, 0).Format("02.01.2006 15:04:05"))
			if childBot.LastError != "" {
				fmt.Fprintf(&statusText, "Причина: %s\n", childBot.LastError)
			}
			if childBot.Status == databaseModels.ChildStatusRunning {
				fmt.Fprintf(&statusText, "Проверять здесь: @%s\n", childBotUsername)
			}
		}

		if activeOwnerID, isRunning := childLauncher.ActiveOwner(); isRunning && activeOwnerID != from.ID {
			fmt.Fprintf(&statusText, "\nТестовый токен сейчас занят ботом администратора %d, новый архив остановит его.", activeOwnerID)
		}
		sendText(ctx, tgBot, chatID, statusText.String())
	}
}
