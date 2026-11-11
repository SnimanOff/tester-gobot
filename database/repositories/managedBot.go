package repositories

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/SnimanOff/testbot-platform/database/models"
	"github.com/redis/go-redis/v9"
)

var (
	ErrorManagedBotNotFound = errors.New("managed bot not found")
	ErrorNoFreeNumber       = errors.New("no free managed bot number")
)

type ManagedBotRepository struct {
	redisClient *redis.Client
	limit       int
}

func NewManagedBotRepository(redisClient *redis.Client, limit int) *ManagedBotRepository {
	return &ManagedBotRepository{redisClient: redisClient, limit: limit}
}

func (repository *ManagedBotRepository) Save(ctx context.Context, managedBot models.ManagedBot) (models.ManagedBot, error) {
	existingBot, err := repository.GetByBotID(ctx, managedBot.BotID)
	if err == nil {
		managedBot.Number = existingBot.Number
	} else if !errors.Is(err, ErrorManagedBotNotFound) {
		return models.ManagedBot{}, err
	} else {
		freeNumber, err := repository.freeNumber(ctx)
		if err != nil {
			return models.ManagedBot{}, err
		}
		managedBot.Number = freeNumber
	}

	_, err = repository.redisClient.TxPipelined(ctx, func(pipeline redis.Pipeliner) error {
		pipeline.HSet(ctx, models.ManagedBotKey(managedBot.Number), managedBot)
		pipeline.HSet(ctx, models.ManagedBotNumbersKey, managedBot.BotID, managedBot.Number)
		return nil
	})
	if err != nil {
		return models.ManagedBot{}, err
	}
	return managedBot, nil
}

func (repository *ManagedBotRepository) GetByNumber(ctx context.Context, number int) (models.ManagedBot, error) {
	var managedBot models.ManagedBot

	botCommand := repository.redisClient.HGetAll(ctx, models.ManagedBotKey(number))
	if err := botCommand.Err(); err != nil {
		return models.ManagedBot{}, err
	}
	if len(botCommand.Val()) == 0 {
		return models.ManagedBot{}, ErrorManagedBotNotFound
	}
	if err := botCommand.Scan(&managedBot); err != nil {
		return models.ManagedBot{}, err
	}
	return managedBot, nil
}

func (repository *ManagedBotRepository) GetByBotID(ctx context.Context, botID int64) (models.ManagedBot, error) {
	storedNumber, err := repository.redisClient.HGet(ctx, models.ManagedBotNumbersKey, strconv.FormatInt(botID, 10)).Result()
	if errors.Is(err, redis.Nil) {
		return models.ManagedBot{}, ErrorManagedBotNotFound
	}
	if err != nil {
		return models.ManagedBot{}, err
	}
	number, err := strconv.Atoi(storedNumber)
	if err != nil {
		return models.ManagedBot{}, err
	}
	return repository.GetByNumber(ctx, number)
}

func (repository *ManagedBotRepository) GetByUsername(ctx context.Context, username string) (models.ManagedBot, error) {
	managedBots, err := repository.List(ctx)
	if err != nil {
		return models.ManagedBot{}, err
	}
	for _, managedBot := range managedBots {
		if strings.EqualFold(managedBot.Username, strings.TrimPrefix(username, "@")) {
			return managedBot, nil
		}
	}
	return models.ManagedBot{}, ErrorManagedBotNotFound
}

func (repository *ManagedBotRepository) List(ctx context.Context) ([]models.ManagedBot, error) {
	var managedBots []models.ManagedBot
	for number := 1; number <= repository.limit; number++ {
		managedBot, err := repository.GetByNumber(ctx, number)
		if errors.Is(err, ErrorManagedBotNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		managedBots = append(managedBots, managedBot)
	}
	return managedBots, nil
}

func (repository *ManagedBotRepository) freeNumber(ctx context.Context) (int, error) {
	for number := 1; number <= repository.limit; number++ {
		exists, err := repository.redisClient.Exists(ctx, models.ManagedBotKey(number)).Result()
		if err != nil {
			return 0, err
		}
		if exists == 0 {
			return number, nil
		}
	}
	return 0, ErrorNoFreeNumber
}
