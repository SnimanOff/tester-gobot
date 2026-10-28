package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/SnimanOff/testbot-platform/bot/tester/router"
	"github.com/SnimanOff/testbot-platform/database"
	"github.com/SnimanOff/testbot-platform/database/repositories"
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

	mainDatabase, err := database.Open(ctx, database.ConnectionOptions{
		Address:  settings.RedisAddress,
		Password: settings.RedisPassword,
		Database: settings.RedisDatabase,
	})
	if err != nil {
		return fmt.Errorf("open main database: %w", err)
	}
	defer mainDatabase.Close()
	fmt.Println("database is started")

	userRepository := repositories.NewUserRepository(mainDatabase)
	if err := userRepository.SetAdmin(ctx, settings.OwnerTelegramID, true); err != nil {
		return fmt.Errorf("set owner admin: %w", err)
	}

	tgBot, err := telegram.New(settings.TesterBotToken)
	if err != nil {
		return fmt.Errorf("create telegram bot: %w", err)
	}

	testerUser, err := tgBot.GetMe(ctx)
	if err != nil {
		return fmt.Errorf("get tester bot info: %w", err)
	}

	router.Register(tgBot, router.Dependencies{
		UserRepository:  userRepository,
		OwnerTelegramID: settings.OwnerTelegramID,
	})

	fmt.Printf("Bot @%s is started\n", testerUser.Username)
	tgBot.Start(ctx)
	fmt.Println("Bot is stopped")
	return nil
}
