package repositories

import (
	"context"
	"errors"
	"testing"

	"github.com/SnimanOff/testbot-platform/database/models"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newTestRepository(test *testing.T) *UserRepository {
	test.Helper()
	fakeRedis := miniredis.RunT(test)
	redisClient := redis.NewClient(&redis.Options{Addr: fakeRedis.Addr()})
	test.Cleanup(func() { redisClient.Close() })
	return NewUserRepository(redisClient)
}

func TestUpsertKeepsAdminMark(test *testing.T) {
	ctx := context.Background()
	userRepository := newTestRepository(test)
	const telegramID int64 = 1001

	if err := userRepository.SetAdmin(ctx, telegramID, true); err != nil {
		test.Fatal(err)
	}
	if err := userRepository.Upsert(ctx, models.User{TelegramID: telegramID, Username: "Tester", FirstName: "Ivan"}); err != nil {
		test.Fatal(err)
	}

	savedUser, err := userRepository.GetByTelegramID(ctx, telegramID)
	if err != nil {
		test.Fatalf("profile not found: %v", err)
	}
	if !savedUser.IsAdmin || savedUser.Username != "Tester" || savedUser.RegisteredAt == 0 {
		test.Fatalf("wrong profile: %+v", savedUser)
	}

	foundID, err := userRepository.GetIDByUsername(ctx, "@tester")
	if err != nil || foundID != telegramID {
		test.Fatalf("get by username: id=%d err=%v", foundID, err)
	}
	if _, err := userRepository.GetIDByUsername(ctx, "@unknown"); !errors.Is(err, ErrorUserNotFound) {
		test.Fatalf("expected ErrorUserNotFound, got: %v", err)
	}
}

func TestAdminMarkAndList(test *testing.T) {
	ctx := context.Background()
	userRepository := newTestRepository(test)

	for _, telegramID := range []int64{30, 10, 20} {
		if err := userRepository.SetAdmin(ctx, telegramID, true); err != nil {
			test.Fatal(err)
		}
	}
	if err := userRepository.SetAdmin(ctx, 20, false); err != nil {
		test.Fatal(err)
	}

	isAdmin, err := userRepository.IsAdmin(ctx, 20)
	if err != nil || isAdmin {
		test.Fatalf("admin mark not removed: isAdmin=%v err=%v", isAdmin, err)
	}
	isAdmin, err = userRepository.IsAdmin(ctx, 999)
	if err != nil || isAdmin {
		test.Fatalf("unknown user became admin: %v %v", isAdmin, err)
	}

	admins, err := userRepository.ListAdmins(ctx)
	if err != nil {
		test.Fatal(err)
	}
	if len(admins) != 2 || admins[0].TelegramID != 10 || admins[1].TelegramID != 30 {
		test.Fatalf("wrong admin list: %+v", admins)
	}
}
