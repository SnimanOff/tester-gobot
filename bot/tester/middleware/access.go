package middleware

import (
	"context"
	"log"

	"github.com/SnimanOff/testbot-platform/database/repositories"
	telegram "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func OnlyAdmins(userRepository *repositories.UserRepository) telegram.Middleware {
	return func(next telegram.HandlerFunc) telegram.HandlerFunc {
		return func(ctx context.Context, tgBot *telegram.Bot, update *models.Update) {
			if update.Message == nil || update.Message.From == nil {
				return
			}

			from := update.Message.From

			isAdmin, err := userRepository.IsAdmin(ctx, from.ID)
			if err != nil {
				log.Printf("check admin %d: %v", from.ID, err)
				denyAccess(ctx, tgBot, update.Message.Chat.ID, "Не удалось проверить права, попробуйте позже.")
				return
			}
			if !isAdmin {
				denyAccess(ctx, tgBot, update.Message.Chat.ID, "Это доступно только администраторам.")
				return
			}
			next(ctx, tgBot, update)
		}
	}
}

func OnlyOwner(ownerTelegramID int64) telegram.Middleware {
	return func(next telegram.HandlerFunc) telegram.HandlerFunc {
		return func(ctx context.Context, tgBot *telegram.Bot, update *models.Update) {
			if update.Message == nil || update.Message.From == nil {
				return
			}
			if update.Message.From.ID != ownerTelegramID {
				denyAccess(ctx, tgBot, update.Message.Chat.ID, "Это доступно только владельцу бота.")
				return
			}
			next(ctx, tgBot, update)
		}
	}
}

func denyAccess(ctx context.Context, tgBot *telegram.Bot, chatID int64, text string) {
	_, err := tgBot.SendMessage(ctx, &telegram.SendMessageParams{
		ChatID: chatID,
		Text:   text,
	})
	if err != nil {
		log.Printf("send access denied to %d: %v", chatID, err)
	}
}
