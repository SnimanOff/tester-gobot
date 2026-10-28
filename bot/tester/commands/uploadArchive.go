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
	"strings"

	"github.com/SnimanOff/testbot-platform/bot/children/archive"
	"github.com/SnimanOff/testbot-platform/bot/children/launcher"
	telegram "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func UploadArchiveCommand(childLauncher *launcher.Launcher, maxArchiveBytes int64, childBotUsername string) telegram.HandlerFunc {
	return func(ctx context.Context, tgBot *telegram.Bot, update *models.Update) {
		if update.Message == nil || update.Message.From == nil || update.Message.Document == nil {
			return
		}

		from := update.Message.From
		chatID := update.Message.Chat.ID
		document := update.Message.Document

		if !strings.EqualFold(filepath.Ext(document.FileName), ".zip") {
			sendText(ctx, tgBot, chatID, "Пришлите бота .zip-архивом, в корне архива должен лежать main.go.")
			return
		}
		if document.FileSize > maxArchiveBytes {
			sendText(ctx, tgBot, chatID, fmt.Sprintf("Архив больше %d МБ.", maxArchiveBytes/(1024*1024)))
			return
		}

		archiveFile, err := os.CreateTemp("", "child-*.zip")
		if err != nil {
			log.Printf("create temp archive: %v", err)
			sendText(ctx, tgBot, chatID, "Не удалось принять архив, попробуйте позже.")
			return
		}
		archivePath := archiveFile.Name()
		_ = archiveFile.Close()
		defer os.Remove(archivePath)

		if err := downloadDocument(ctx, tgBot, document.FileID, archivePath, maxArchiveBytes); err != nil {
			log.Printf("download archive from %d: %v", from.ID, err)
			sendText(ctx, tgBot, chatID, "Не удалось скачать архив из Telegram, попробуйте еще раз.")
			return
		}
		sendText(ctx, tgBot, chatID, "Архив получен: проверяю на zip-бомбу, собираю и запускаю...")

		if err := childLauncher.Deploy(ctx, from.ID, archivePath, document.FileName); err != nil {
			sendHTML(ctx, tgBot, chatID, describeDeployError(err))
			return
		}
		sendText(ctx, tgBot, chatID, fmt.Sprintf("Дочерний бот запущен на тестовом токене: @%s\nОстановить: /stop", childBotUsername))
	}
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
		return "Архив отклонен: " + html.EscapeString(rejectedError.Reason)
	}

	var launchError *launcher.LaunchError
	if errors.As(deployError, &launchError) {
		errorText := fmt.Sprintf("Бот не запустился. Этап: %s.\n%s",
			html.EscapeString(launchError.Stage), html.EscapeString(launchError.Reason))
		if launchError.Output != "" {
			errorText += "\n<pre>" + html.EscapeString(launchError.Output) + "</pre>"
		}
		return errorText
	}

	log.Printf("deploy child bot: %v", deployError)
	return "Не удалось запустить бота из-за внутренней ошибки, попробуйте позже."
}
