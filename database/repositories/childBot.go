package repositories

import (
	"context"
	"errors"
	"time"

	"github.com/SnimanOff/testbot-platform/database/models"
	"github.com/redis/go-redis/v9"
)

var ErrorChildBotNotFound = errors.New("child bot not found")

type ChildBotRepository struct {
	redisClient *redis.Client
}

func NewChildBotRepository(redisClient *redis.Client) *ChildBotRepository {
	return &ChildBotRepository{redisClient: redisClient}
}

func (repository *ChildBotRepository) Upsert(ctx context.Context, childBot models.ChildBot) error {
	childBot.UpdatedAt = time.Now().Unix()
	return repository.redisClient.HSet(ctx, models.ChildBotKey(childBot.OwnerID), childBot).Err()
}

func (repository *ChildBotRepository) GetByOwnerID(ctx context.Context, ownerID int64) (models.ChildBot, error) {
	var childBot models.ChildBot

	childBotCommand := repository.redisClient.HGetAll(ctx, models.ChildBotKey(ownerID))
	if err := childBotCommand.Err(); err != nil {
		return models.ChildBot{}, err
	}
	if len(childBotCommand.Val()) == 0 {
		return models.ChildBot{}, ErrorChildBotNotFound
	}
	if err := childBotCommand.Scan(&childBot); err != nil {
		return models.ChildBot{}, err
	}
	return childBot, nil
}

func (repository *ChildBotRepository) MarkInterruptedAsStopped(ctx context.Context) error {
	keyIterator := repository.redisClient.Scan(ctx, 0, models.ChildBotKeyPattern, 100).Iterator()
	for keyIterator.Next(ctx) {
		childBotKey := keyIterator.Val()
		childStatus, err := repository.redisClient.HGet(ctx, childBotKey, models.ChildFieldStatus).Result()
		if err != nil && !errors.Is(err, redis.Nil) {
			return err
		}
		if childStatus == models.ChildStatusFailed || childStatus == models.ChildStatusStopped {
			continue
		}
		err = repository.redisClient.HSet(ctx, childBotKey,
			models.ChildFieldStatus, models.ChildStatusStopped,
			models.ChildFieldLastError, "основной бот был перезапущен",
			models.ChildFieldUpdatedAt, time.Now().Unix(),
		).Err()
		if err != nil {
			return err
		}
	}
	return keyIterator.Err()
}
