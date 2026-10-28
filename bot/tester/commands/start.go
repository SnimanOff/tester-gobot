package commands

import (
	"context"
	"fmt"
	"log"
	"strings"

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
			sendText(ctx, tgBot, chatID, "Не удалось сохранить профиль, попробуйте позже.")
			return
		}

		isAdmin, err := userRepository.IsAdmin(ctx, from.ID)
		if err != nil {
			log.Printf("check admin %d: %v", from.ID, err)
			sendText(ctx, tgBot, chatID, "Не удалось проверить права, попробуйте позже.")
			return
		}

		var startText strings.Builder
		fmt.Fprintf(&startText, "Привет, %s! Я бот-тестировщик: запускаю присланных ботов на тестовом токене.\n", from.FirstName)
		fmt.Fprintf(&startText, "Ваш Telegram ID: %d\n\n", from.ID)

		if !isAdmin {
			startText.WriteString("Присылать ботов могут только администраторы. Передайте свой ID владельцу.")
			sendText(ctx, tgBot, chatID, startText.String())
			return
		}

		startText.WriteString("Пришлите .zip-архив с ботом, в корне архива должен лежать main.go.\n\n")
		startText.WriteString("/status - состояние вашего дочернего бота\n")
		startText.WriteString("/stop - остановить дочернего бота\n")
		startText.WriteString("/admins - список администраторов\n")
		if from.ID == ownerTelegramID {
			startText.WriteString("/add_admin <ID или @username> - назначить администратора\n")
			startText.WriteString("/remove_admin <ID или @username> - снять администратора\n")
		}
		sendText(ctx, tgBot, chatID, startText.String())
	}
}
