package commands

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/SnimanOff/testbot-platform/database/repositories"
	telegram "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func ListAdminsCommand(userRepository *repositories.UserRepository, ownerTelegramID int64) telegram.HandlerFunc {
	return func(ctx context.Context, tgBot *telegram.Bot, update *models.Update) {
		if update.Message == nil {
			return
		}

		chatID := update.Message.Chat.ID

		admins, err := userRepository.ListAdmins(ctx)
		if err != nil {
			log.Printf("list admins: %v", err)
			sendText(ctx, tgBot, chatID, "Не удалось получить список администраторов, попробуйте позже.")
			return
		}
		if len(admins) == 0 {
			sendText(ctx, tgBot, chatID, "Администраторов пока нет.")
			return
		}

		var adminsText strings.Builder
		adminsText.WriteString("Администраторы:\n")
		for _, adminProfile := range admins {
			fmt.Fprintf(&adminsText, "- %d", adminProfile.TelegramID)
			if adminProfile.Username != "" {
				fmt.Fprintf(&adminsText, " @%s", adminProfile.Username)
			}
			if adminProfile.FirstName != "" {
				fmt.Fprintf(&adminsText, " (%s)", adminProfile.FirstName)
			}
			if adminProfile.TelegramID == ownerTelegramID {
				adminsText.WriteString(" - владелец")
			}
			adminsText.WriteString("\n")
		}
		sendText(ctx, tgBot, chatID, adminsText.String())
	}
}
