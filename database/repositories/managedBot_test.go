package repositories

import (
	"context"
	"errors"
	"testing"

	"github.com/SnimanOff/testbot-platform/database/models"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestManagedBotNumbers(test *testing.T) {
	ctx := context.Background()
	fakeRedis := miniredis.RunT(test)
	redisClient := redis.NewClient(&redis.Options{Addr: fakeRedis.Addr()})
	defer redisClient.Close()
	managedBotRepository := NewManagedBotRepository(redisClient, 2)

	firstBot, err := managedBotRepository.Save(ctx, models.ManagedBot{BotID: 100, Username: "first_test_bot", OwnerID: 7, Token: "token-1"})
	if err != nil || firstBot.Number != 1 {
		test.Fatalf("first bot: %+v %v", firstBot, err)
	}
	secondBot, err := managedBotRepository.Save(ctx, models.ManagedBot{BotID: 200, Username: "second_test_bot", OwnerID: 8, Token: "token-2"})
	if err != nil || secondBot.Number != 2 {
		test.Fatalf("second bot: %+v %v", secondBot, err)
	}
	if _, err := managedBotRepository.Save(ctx, models.ManagedBot{BotID: 300, Username: "third_test_bot"}); !errors.Is(err, ErrorNoFreeNumber) {
		test.Fatalf("expected ErrorNoFreeNumber, got %v", err)
	}

	updatedBot, err := managedBotRepository.Save(ctx, models.ManagedBot{BotID: 100, Username: "first_test_bot", OwnerID: 7, Token: "token-1-new"})
	if err != nil || updatedBot.Number != 1 {
		test.Fatalf("token update changed number: %+v %v", updatedBot, err)
	}
	storedBot, err := managedBotRepository.GetByUsername(ctx, "@First_Test_Bot")
	if err != nil || storedBot.Token != "token-1-new" || storedBot.OwnerID != 7 {
		test.Fatalf("stored bot: %+v %v", storedBot, err)
	}
	managedBots, err := managedBotRepository.List(ctx)
	if err != nil || len(managedBots) != 2 {
		test.Fatalf("list: %+v %v", managedBots, err)
	}
	if _, err := managedBotRepository.GetByBotID(ctx, 999); !errors.Is(err, ErrorManagedBotNotFound) {
		test.Fatalf("expected ErrorManagedBotNotFound, got %v", err)
	}
}
