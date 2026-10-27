package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/SnimanOff/testbot-platform/bot/tester/router"
	telegram "github.com/go-telegram/bot"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	settings, err := loadSettings()
	if err != nil {
		return fmt.Errorf("load settings: %w", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	tgBot, err := telegram.New(settings.TesterBotToken)
	if err != nil {
		return fmt.Errorf("create telegram bot: %w", err)
	}

	testerUser, err := tgBot.GetMe(ctx)
	if err != nil {
		return fmt.Errorf("get tester bot info: %w", err)
	}

	router.Register(tgBot)

	fmt.Printf("Bot @%s is started\n", testerUser.Username)
	tgBot.Start(ctx)
	fmt.Println("Bot is stopped")
	return nil
}
