package commands

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/SnimanOff/testbot-platform/bot/children/launcher"
	databaseModels "github.com/SnimanOff/testbot-platform/database/models"
	"github.com/SnimanOff/testbot-platform/database/repositories"
	telegram "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

const newBotButtonText = "Создать тестового бота"

func NewBotCommand() telegram.HandlerFunc {
	return func(ctx context.Context, tgBot *telegram.Bot, update *models.Update) {
		if update.Message == nil {
			return
		}

		_, err := tgBot.SendMessage(ctx, &telegram.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "Нажмите кнопку и подтвердите создание бота.",
			ReplyMarkup: &models.ReplyKeyboardMarkup{
				Keyboard: [][]models.KeyboardButton{{{
					Text: newBotButtonText,
					RequestManagedBot: &models.KeyboardButtonRequestManagedBot{
						RequestID:     int32(update.Message.ID),
						SuggestedName: "Тестовый бот",
					},
				}}},
				ResizeKeyboard:  true,
				OneTimeKeyboard: true,
			},
		})
		if err != nil {
			log.Printf("send new bot button to %d: %v", update.Message.Chat.ID, err)
		}
	}
}

func ManagedBotUpdate(managedBotRepository *repositories.ManagedBotRepository, userRepository *repositories.UserRepository, childLauncher *launcher.Launcher, ownerTelegramID int64) telegram.HandlerFunc {
	return func(ctx context.Context, tgBot *telegram.Bot, update *models.Update) {
		if update.ManagedBot == nil || update.ManagedBot.User == nil || update.ManagedBot.Bot == nil {
			return
		}

		creator := update.ManagedBot.User
		createdBot := update.ManagedBot.Bot

		isAdmin, err := userRepository.IsAdmin(ctx, creator.ID)
		if err != nil {
			log.Printf("check admin %d: %v", creator.ID, err)
			return
		}
		if !isAdmin && creator.ID != ownerTelegramID {
			sendText(ctx, tgBot, creator.ID, "Создавать тестовых ботов могут только администраторы.")
			return
		}

		botToken, err := tgBot.GetManagedBotToken(ctx, &telegram.GetManagedBotTokenParams{UserID: createdBot.ID})
		if err != nil {
			log.Printf("get managed bot %d token: %v", createdBot.ID, err)
			sendText(ctx, tgBot, creator.ID, "Не удалось получить токен бота.")
			return
		}

		managedBot, err := managedBotRepository.Save(ctx, databaseModels.ManagedBot{
			BotID:    createdBot.ID,
			Username: createdBot.Username,
			OwnerID:  creator.ID,
			Token:    botToken,
		})
		if errors.Is(err, repositories.ErrorNoFreeNumber) {
			sendText(ctx, tgBot, creator.ID, fmt.Sprintf("Достигнут предел: %d тестовых ботов.", childLauncher.Slots()))
			return
		}
		if err != nil {
			log.Printf("save managed bot %d: %v", createdBot.ID, err)
			sendText(ctx, tgBot, creator.ID, "Ошибка сохранения бота.")
			return
		}

		if childLauncher.Stop(managedBot.Number) {
			sendText(ctx, tgBot, creator.ID, fmt.Sprintf("Токен бота %d изменился, бот остановлен. Отправьте архив заново.", managedBot.Number))
			return
		}
		sendText(ctx, tgBot, creator.ID, fmt.Sprintf("Бот %d создан: @%s. Отправьте ZIP с main.go, он запустится в отдельном контейнере.", managedBot.Number, managedBot.Username))
	}
}
