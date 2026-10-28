package repositories

import (
	"context"
	"errors"
	"testing"

	"github.com/SnimanOff/testbot-platform/database/models"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestUpsertGetAndMarkInterrupted(test *testing.T) {
	ctx := context.Background()
	fakeRedis := miniredis.RunT(test)
	redisClient := redis.NewClient(&redis.Options{Addr: fakeRedis.Addr()})
	defer redisClient.Close()
	childRepository := NewChildBotRepository(redisClient)

	runningChild := models.ChildBot{OwnerID: 1, ArchiveName: "echo.zip", Status: models.ChildStatusRunning}
	failedChild := models.ChildBot{OwnerID: 2, ArchiveName: "broken.zip", Status: models.ChildStatusFailed, LastError: "build error"}
	for _, childBot := range []models.ChildBot{runningChild, failedChild} {
		if err := childRepository.Upsert(ctx, childBot); err != nil {
			test.Fatal(err)
		}
	}

	if err := childRepository.MarkInterruptedAsStopped(ctx); err != nil {
		test.Fatal(err)
	}

	stoppedChild, err := childRepository.GetByOwnerID(ctx, 1)
	if err != nil || stoppedChild.Status != models.ChildStatusStopped || stoppedChild.ArchiveName != "echo.zip" {
		test.Fatalf("running bot not marked stopped: %+v %v", stoppedChild, err)
	}
	unchangedChild, err := childRepository.GetByOwnerID(ctx, 2)
	if err != nil || unchangedChild.Status != models.ChildStatusFailed || unchangedChild.LastError != "build error" {
		test.Fatalf("failed bot status changed: %+v %v", unchangedChild, err)
	}
	if _, err := childRepository.GetByOwnerID(ctx, 3); !errors.Is(err, ErrorChildBotNotFound) {
		test.Fatalf("expected ErrorChildBotNotFound, got: %v", err)
	}
}
