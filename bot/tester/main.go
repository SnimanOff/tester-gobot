package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/SnimanOff/testbot-platform/bot/children/archive"
	"github.com/SnimanOff/testbot-platform/bot/children/launcher"
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

	childDatabase, err := database.Open(ctx, database.ConnectionOptions{
		Address:  settings.ChildRedisAddress,
		Password: settings.ChildRedisPassword,
		Database: settings.ChildRedisDatabase,
	})
	if err != nil {
		return fmt.Errorf("open child database: %w", err)
	}
	defer childDatabase.Close()
	fmt.Println("database is started")

	userRepository := repositories.NewUserRepository(mainDatabase)
	if err := userRepository.SetAdmin(ctx, settings.OwnerTelegramID, true); err != nil {
		return fmt.Errorf("set owner admin: %w", err)
	}
	childRepository := repositories.NewChildBotRepository(mainDatabase)
	if err := childRepository.MarkInterruptedAsStopped(ctx); err != nil {
		return fmt.Errorf("reset child statuses: %w", err)
	}

	childBotUsername, err := fetchBotUsername(ctx, settings.ChildBotToken)
	if err != nil {
		return fmt.Errorf("check child bot token: %w", err)
	}

	tgBot, err := telegram.New(settings.TesterBotToken)
	if err != nil {
		return fmt.Errorf("create telegram bot: %w", err)
	}

	testerUser, err := tgBot.GetMe(ctx)
	if err != nil {
		return fmt.Errorf("get tester bot info: %w", err)
	}

	childLauncher, err := launcher.New(launcher.Options{
		ChildrenDirectory: settings.ChildrenDirectory,
		ChildBotToken:     settings.ChildBotToken,
		ChildRedis: launcher.ChildRedis{
			Address:  settings.ChildRedisAddress,
			Password: settings.ChildRedisPassword,
			Database: settings.ChildRedisDatabase,
		},
		ChildUser: launcher.ChildUser{UID: settings.ChildRunUID, GID: settings.ChildRunGID},
		ArchiveLimits: archive.Limits{
			MaxArchiveBytes:     settings.MaxArchiveBytes,
			MaxUnpackedBytes:    settings.MaxUnpackedBytes,
			MaxFiles:            settings.MaxArchiveFiles,
			MaxCompressionRatio: settings.MaxCompressionRatio,
		},
		BuildTimeout: settings.ChildBuildTimeout,
		StartupCheck: settings.ChildStartupCheck,
	}, childRepository, childDatabase, func(ownerID int64, exitReport string) {
		_, err := tgBot.SendMessage(context.Background(), &telegram.SendMessageParams{
			ChatID: ownerID,
			Text:   "Дочерний бот остановился: " + exitReport,
		})
		if err != nil {
			log.Printf("notify child exit %d: %v", ownerID, err)
		}
	})
	if err != nil {
		return fmt.Errorf("create child launcher: %w", err)
	}
	defer childLauncher.StopActive()

	router.Register(tgBot, router.Dependencies{
		UserRepository:   userRepository,
		ChildRepository:  childRepository,
		ChildLauncher:    childLauncher,
		OwnerTelegramID:  settings.OwnerTelegramID,
		ChildBotUsername: childBotUsername,
		MaxArchiveBytes:  settings.MaxArchiveBytes,
	})

	fmt.Printf("Bot @%s is started, children run on @%s\n", testerUser.Username, childBotUsername)
	tgBot.Start(ctx)
	fmt.Println("Bot is stopped")
	return nil
}

func fetchBotUsername(ctx context.Context, botToken string) (string, error) {
	tokenCheckBot, err := telegram.New(botToken, telegram.WithSkipGetMe())
	if err != nil {
		return "", err
	}

	botUser, err := tokenCheckBot.GetMe(ctx)
	if err != nil {
		return "", err
	}
	return botUser.Username, nil
}
