package router

import (
	"github.com/SnimanOff/testbot-platform/bot/tester/commands"
	"github.com/SnimanOff/testbot-platform/bot/tester/middleware"
	"github.com/SnimanOff/testbot-platform/database/repositories"
	telegram "github.com/go-telegram/bot"
)

type Dependencies struct {
	UserRepository  *repositories.UserRepository
	OwnerTelegramID int64
}

func Register(tgBot *telegram.Bot, dependencies Dependencies) {
	onlyAdmins := middleware.OnlyAdmins(dependencies.UserRepository)
	onlyOwner := middleware.OnlyOwner(dependencies.OwnerTelegramID)

	tgBot.RegisterHandler(
		telegram.HandlerTypeMessageText,
		"start",
		telegram.MatchTypeCommandStartOnly,
		commands.StartCommand(dependencies.UserRepository, dependencies.OwnerTelegramID),
	)

	tgBot.RegisterHandler(
		telegram.HandlerTypeMessageText,
		"admins",
		telegram.MatchTypeCommandStartOnly,
		commands.ListAdminsCommand(dependencies.UserRepository, dependencies.OwnerTelegramID),
		onlyAdmins,
	)

	tgBot.RegisterHandler(
		telegram.HandlerTypeMessageText,
		"add_admin",
		telegram.MatchTypeCommandStartOnly,
		commands.AddAdminCommand(dependencies.UserRepository),
		onlyOwner,
	)

	tgBot.RegisterHandler(
		telegram.HandlerTypeMessageText,
		"remove_admin",
		telegram.MatchTypeCommandStartOnly,
		commands.RemoveAdminCommand(dependencies.UserRepository, dependencies.OwnerTelegramID),
		onlyOwner,
	)
}
