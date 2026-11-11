package commands

import (
	"context"
	"fmt"
	"log"

	databaseModels "github.com/SnimanOff/testbot-platform/database/models"
	"github.com/SnimanOff/testbot-platform/database/repositories"
	telegram "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func StartCommand(userRepository *repositories.UserRepository, ownerTelegramID int64) telegram.HandlerFunc {
	return func(ctx context.Context, tgBot *telegram.Bot, update *models.Update) {
		if update.Message == nil || update.Message.From == nil {
			return
		}

		from := update.Message.From
		chatID := update.Message.Chat.ID

		err := userRepository.Upsert(ctx, databaseModels.User{
			TelegramID: from.ID,
			Username:   from.Username,
			FirstName:  from.FirstName,
		})
		if err != nil {
			log.Printf("upsert user %d: %v", from.ID, err)
			sendText(ctx, tgBot, chatID, "Ошибка сохранения профиля.")
			return
		}

		isAdmin, err := userRepository.IsAdmin(ctx, from.ID)
		if err != nil {
			log.Printf("check admin %d: %v", from.ID, err)
			sendText(ctx, tgBot, chatID, "Ошибка проверки прав.")
			return
		}

		startText := fmt.Sprintf("ID: %d\n", from.ID)
		if !isAdmin {
			sendText(ctx, tgBot, chatID, startText+"Загрузка доступна администраторам.")
			return
		}
		startText += "/newbot - создать тестового бота\nЗатем отправьте ZIP с main.go в корне.\n/status\n/stop\n/admins"
		if from.ID == ownerTelegramID {
			startText += "\n/add_admin <ID или @username>\n/remove_admin <ID или @username>"
		}
		sendText(ctx, tgBot, chatID, startText)
	}
}
