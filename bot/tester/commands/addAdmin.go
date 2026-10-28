package commands

import (
	"context"
	"fmt"
	"log"

	"github.com/SnimanOff/testbot-platform/database/repositories"
	telegram "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func AddAdminCommand(userRepository *repositories.UserRepository) telegram.HandlerFunc {
	return func(ctx context.Context, tgBot *telegram.Bot, update *models.Update) {
		if update.Message == nil {
			return
		}

		chatID := update.Message.Chat.ID

		targetID, err := resolveTargetUser(ctx, userRepository, update.Message)
		if isTargetError(err) {
			sendText(ctx, tgBot, chatID, err.Error())
			return
		}
		if err != nil {
			log.Printf("resolve /add_admin target: %v", err)
			sendText(ctx, tgBot, chatID, "Не удалось найти пользователя, попробуйте позже.")
			return
		}

		if err := userRepository.SetAdmin(ctx, targetID, true); err != nil {
			log.Printf("set admin %d: %v", targetID, err)
			sendText(ctx, tgBot, chatID, "Не удалось назначить администратора, попробуйте позже.")
			return
		}
		sendText(ctx, tgBot, chatID, fmt.Sprintf("Пользователь %d назначен администратором.", targetID))
	}
}
