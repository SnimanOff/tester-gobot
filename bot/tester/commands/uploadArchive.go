package commands

import (
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/SnimanOff/testbot-platform/bot/children/archive"
	"github.com/SnimanOff/testbot-platform/bot/children/launcher"
	databaseModels "github.com/SnimanOff/testbot-platform/database/models"
	"github.com/SnimanOff/testbot-platform/database/repositories"
	telegram "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func UploadArchiveCommand(childLauncher *launcher.Launcher, managedBotRepository *repositories.ManagedBotRepository, maxArchiveBytes int64, ownerTelegramID int64) telegram.HandlerFunc {
	return func(ctx context.Context, tgBot *telegram.Bot, update *models.Update) {
		if update.Message == nil || update.Message.From == nil || update.Message.Document == nil {
			return
		}

		from := update.Message.From
		chatID := update.Message.Chat.ID
		document := update.Message.Document

		if !strings.EqualFold(filepath.Ext(document.FileName), ".zip") {
			sendText(ctx, tgBot, chatID, "Отправьте ZIP с main.go в корне.")
			return
		}
		if document.FileSize > maxArchiveBytes {
			sendText(ctx, tgBot, chatID, fmt.Sprintf("Архив больше %d МБ.", maxArchiveBytes/(1024*1024)))
			return
		}

		managedBot, choiceProblem := chooseManagedBot(ctx, managedBotRepository, childLauncher, update.Message.Caption, from.ID, ownerTelegramID)
		if choiceProblem != "" {
			sendText(ctx, tgBot, chatID, choiceProblem)
			return
		}

		archiveFile, err := os.CreateTemp("", "child-*.zip")
		if err != nil {
			log.Printf("create temp archive: %v", err)
			sendText(ctx, tgBot, chatID, "Ошибка загрузки.")
			return
		}
		archivePath := archiveFile.Name()
		_ = archiveFile.Close()
		defer os.Remove(archivePath)

		if err := downloadDocument(ctx, tgBot, document.FileID, archivePath, maxArchiveBytes); err != nil {
			log.Printf("download archive from %d: %v", from.ID, err)
			sendText(ctx, tgBot, chatID, "Ошибка загрузки.")
			return
		}
		sendText(ctx, tgBot, chatID, "Загрузка...")

		if err := childLauncher.Deploy(ctx, from.ID, managedBot.Number, managedBot.Token, archivePath, document.FileName); err != nil {
			sendHTML(ctx, tgBot, chatID, describeDeployError(err))
			return
		}
		sendText(ctx, tgBot, chatID, fmt.Sprintf("Бот %d запущен: @%s", managedBot.Number, managedBot.Username))
	}
}

func chooseManagedBot(ctx context.Context, managedBotRepository *repositories.ManagedBotRepository, childLauncher *launcher.Launcher, caption string, userID, ownerTelegramID int64) (databaseModels.ManagedBot, string) {
	caption = strings.TrimSpace(caption)
	if caption != "" {
		var managedBot databaseModels.ManagedBot
		var err error
		if number, parseError := strconv.Atoi(caption); parseError == nil {
			managedBot, err = managedBotRepository.GetByNumber(ctx, number)
		} else {
			managedBot, err = managedBotRepository.GetByUsername(ctx, caption)
		}
		if errors.Is(err, repositories.ErrorManagedBotNotFound) {
			return databaseModels.ManagedBot{}, "Бот не найден. Список ботов: /status"
		}
		if err != nil {
			log.Printf("find managed bot %q: %v", caption, err)
			return databaseModels.ManagedBot{}, "Ошибка поиска бота."
		}
		if managedBot.OwnerID != userID && userID != ownerTelegramID {
			return databaseModels.ManagedBot{}, "Это бот другого администратора."
		}
		return managedBot, ""
	}

	managedBots, err := managedBotRepository.List(ctx)
	if err != nil {
		log.Printf("list managed bots: %v", err)
		return databaseModels.ManagedBot{}, "Ошибка поиска бота."
	}
	hasOwnBots := false
	for _, managedBot := range managedBots {
		if managedBot.OwnerID != userID {
			continue
		}
		hasOwnBots = true
		if _, isRunning := childLauncher.SlotOwner(managedBot.Number); !isRunning {
			return managedBot, ""
		}
	}
	if hasOwnBots {
		return databaseModels.ManagedBot{}, "Все ваши боты запущены. Укажите номер или @username бота в подписи к архиву."
	}
	return databaseModels.ManagedBot{}, "Сначала создайте бота: /newbot"
}

func downloadDocument(ctx context.Context, tgBot *telegram.Bot, fileID string, destinationPath string, maxBytes int64) error {
	telegramFile, err := tgBot.GetFile(ctx, &telegram.GetFileParams{FileID: fileID})
	if err != nil {
		return fmt.Errorf("get file: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, tgBot.FileDownloadLink(telegramFile), nil)
	if err != nil {
		return err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return fmt.Errorf("download file: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("download file: status %d", response.StatusCode)
	}

	destinationFile, err := os.Create(destinationPath)
	if err != nil {
		return err
	}
	defer destinationFile.Close()

	_, err = io.Copy(destinationFile, io.LimitReader(response.Body, maxBytes+1))
	return err
}

func describeDeployError(deployError error) string {
	var rejectedError *archive.RejectedError
	if errors.As(deployError, &rejectedError) {
		log.Printf("reject archive: %s", rejectedError.Reason)
		return "Ошибка загрузки архива."
	}

	var launchError *launcher.LaunchError
	if errors.As(deployError, &launchError) {
		errorText := fmt.Sprintf("Ошибка: %s.\n%s",
			html.EscapeString(launchError.Stage), html.EscapeString(launchError.Reason))
		if launchError.Output != "" {
			errorText += "\n<pre>" + html.EscapeString(launchError.Output) + "</pre>"
		}
		return errorText
	}

	log.Printf("deploy child bot: %v", deployError)
	return "Ошибка запуска."
}
