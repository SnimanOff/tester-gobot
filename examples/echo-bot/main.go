package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"

	telegram "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/redis/go-redis/v9"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	redisDatabase, err := strconv.Atoi(os.Getenv("REDIS_DB"))
	if err != nil {
		return fmt.Errorf("parse REDIS_DB: %w", err)
	}

	redisClient := redis.NewClient(&redis.Options{
		Addr:     os.Getenv("REDIS_ADDR"),
		Password: os.Getenv("REDIS_PASSWORD"),
		DB:       redisDatabase,
	})
	if err := redisClient.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("connect redis: %w", err)
	}
	defer redisClient.Close()

	echoBot, err := telegram.New(os.Getenv("BOT_TOKEN"), telegram.WithDefaultHandler(echoHandler(redisClient)))
	if err != nil {
		return fmt.Errorf("create telegram bot: %w", err)
	}

	botUser, err := echoBot.GetMe(ctx)
	if err != nil {
		return fmt.Errorf("get bot info: %w", err)
	}

	fmt.Printf("Echo bot @%s is started\n", botUser.Username)
	echoBot.Start(ctx)
	return nil
}

func echoHandler(redisClient *redis.Client) telegram.HandlerFunc {
	return func(ctx context.Context, tgBot *telegram.Bot, update *models.Update) {
		if update.Message == nil || update.Message.From == nil || update.Message.Text == "" {
			return
		}

		counterKey := "messages:" + strconv.FormatInt(update.Message.From.ID, 10)
		messageCount, err := redisClient.Incr(ctx, counterKey).Result()
		if err != nil {
			log.Printf("increment counter: %v", err)
			return
		}

		_, err = tgBot.SendMessage(ctx, &telegram.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   fmt.Sprintf("%s\n\nсообщение №%d", update.Message.Text, messageCount),
		})
		if err != nil {
			log.Printf("send echo: %v", err)
		}
	}
}
