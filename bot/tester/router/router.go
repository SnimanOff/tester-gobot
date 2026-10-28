package router

import (
	"github.com/SnimanOff/testbot-platform/bot/tester/commands"
	"github.com/SnimanOff/testbot-platform/database/repositories"
	telegram "github.com/go-telegram/bot"
)

type Dependencies struct {
	UserRepository  *repositories.UserRepository
	OwnerTelegramID int64
}

func Register(tgBot *telegram.Bot, dependencies Dependencies) {
	tgBot.RegisterHandler(
		telegram.HandlerTypeMessageText,
		"start",
		telegram.MatchTypeCommandStartOnly,
		commands.StartCommand(dependencies.UserRepository, dependencies.OwnerTelegramID),
	)
}
