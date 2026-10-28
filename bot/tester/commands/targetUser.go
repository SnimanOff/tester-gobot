package commands

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/SnimanOff/testbot-platform/database/repositories"
	"github.com/go-telegram/bot/models"
)

var (
	ErrorTargetNotGiven  = errors.New("укажите Telegram ID или @username, либо ответьте командой на сообщение пользователя")
	ErrorUsernameUnknown = errors.New("пользователь с таким username не найден: он должен сначала нажать /start")
)

func resolveTargetUser(ctx context.Context, userRepository *repositories.UserRepository, message *models.Message) (int64, error) {
	commandArguments := strings.Fields(message.Text)[1:]
	if len(commandArguments) == 0 {
		repliedMessage := message.ReplyToMessage
		if repliedMessage != nil && repliedMessage.From != nil {
			return repliedMessage.From.ID, nil
		}
		return 0, ErrorTargetNotGiven
	}

	targetArgument := commandArguments[0]
	if telegramID, err := strconv.ParseInt(targetArgument, 10, 64); err == nil {
		return telegramID, nil
	}
	if !strings.HasPrefix(targetArgument, "@") {
		return 0, ErrorTargetNotGiven
	}

	telegramID, err := userRepository.GetIDByUsername(ctx, targetArgument)
	if errors.Is(err, repositories.ErrorUserNotFound) {
		return 0, ErrorUsernameUnknown
	}
	if err != nil {
		return 0, err
	}
	return telegramID, nil
}

func isTargetError(err error) bool {
	return errors.Is(err, ErrorTargetNotGiven) || errors.Is(err, ErrorUsernameUnknown)
}
