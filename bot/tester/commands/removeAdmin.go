package commands

import (
	"context"
	"fmt"
	"log"

	"github.com/SnimanOff/testbot-platform/database/repositories"
	telegram "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func RemoveAdminCommand(userRepository *repositories.UserRepository, ownerTelegramID int64) telegram.HandlerFunc {
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
			log.Printf("resolve /remove_admin target: %v", err)
			sendText(ctx, tgBot, chatID, "Не удалось найти пользователя, попробуйте позже.")
			return
		}
		if targetID == ownerTelegramID {
			sendText(ctx, tgBot, chatID, "Владельца нельзя снять с администраторов.")
			return
		}

		if err := userRepository.SetAdmin(ctx, targetID, false); err != nil {
			log.Printf("unset admin %d: %v", targetID, err)
			sendText(ctx, tgBot, chatID, "Не удалось снять администратора, попробуйте позже.")
			return
		}
		sendText(ctx, tgBot, chatID, fmt.Sprintf("Пользователь %d больше не администратор.", targetID))
	}
}
