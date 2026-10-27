package router

import (
	"github.com/SnimanOff/testbot-platform/bot/tester/commands"
	telegram "github.com/go-telegram/bot"
)

func Register(tgBot *telegram.Bot) {
	tgBot.RegisterHandler(
		telegram.HandlerTypeMessageText,
		"start",
		telegram.MatchTypeCommandStartOnly,
		commands.StartCommand(),
	)
}
