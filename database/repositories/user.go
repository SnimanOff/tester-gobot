package repositories

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/SnimanOff/testbot-platform/database/models"
	"github.com/redis/go-redis/v9"
)

var ErrorUserNotFound = errors.New("user not found")

const (
	adminMarkSet   = "1"
	adminMarkUnset = "0"
)

type UserRepository struct {
	redisClient *redis.Client
}

func NewUserRepository(redisClient *redis.Client) *UserRepository {
	return &UserRepository{redisClient: redisClient}
}

func (repository *UserRepository) Upsert(ctx context.Context, user models.User) error {
	userKey := models.UserKey(user.TelegramID)

	previousUsername, err := repository.redisClient.HGet(ctx, userKey, models.UserFieldUsername).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return err
	}

	_, err = repository.redisClient.TxPipelined(ctx, func(pipeline redis.Pipeliner) error {
		pipeline.HSet(ctx, userKey,
			models.UserFieldTelegramID, user.TelegramID,
			models.UserFieldUsername, user.Username,
			models.UserFieldFirstName, user.FirstName,
		)
		pipeline.HSetNX(ctx, userKey, models.UserFieldRegisteredAt, time.Now().Unix())
		pipeline.HSetNX(ctx, userKey, models.UserFieldIsAdmin, adminMarkUnset)

		if previousUsername != "" && !strings.EqualFold(previousUsername, user.Username) {
			pipeline.Del(ctx, models.UsernameKey(previousUsername))
		}
		if user.Username != "" {
			pipeline.Set(ctx, models.UsernameKey(user.Username), user.TelegramID, 0)
		}
		return nil
	})
	return err
}

func (repository *UserRepository) GetByTelegramID(ctx context.Context, telegramID int64) (models.User, error) {
	var user models.User

	profileCommand := repository.redisClient.HGetAll(ctx, models.UserKey(telegramID))
	if err := profileCommand.Err(); err != nil {
		return models.User{}, err
	}
	if len(profileCommand.Val()) == 0 {
		return models.User{}, ErrorUserNotFound
	}
	if err := profileCommand.Scan(&user); err != nil {
		return models.User{}, err
	}
	return user, nil
}

func (repository *UserRepository) GetIDByUsername(ctx context.Context, username string) (int64, error) {
	storedID, err := repository.redisClient.Get(ctx, models.UsernameKey(username)).Result()
	if errors.Is(err, redis.Nil) {
		return 0, ErrorUserNotFound
	}
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(storedID, 10, 64)
}

func (repository *UserRepository) IsAdmin(ctx context.Context, telegramID int64) (bool, error) {
	adminMark, err := repository.redisClient.HGet(ctx, models.UserKey(telegramID), models.UserFieldIsAdmin).Result()
	if errors.Is(err, redis.Nil) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return adminMark == adminMarkSet, nil
}

func (repository *UserRepository) SetAdmin(ctx context.Context, telegramID int64, isAdmin bool) error {
	userKey := models.UserKey(telegramID)
	_, err := repository.redisClient.TxPipelined(ctx, func(pipeline redis.Pipeliner) error {
		if isAdmin {
			pipeline.HSet(ctx, userKey, models.UserFieldTelegramID, telegramID, models.UserFieldIsAdmin, adminMarkSet)
			pipeline.SAdd(ctx, models.AdminIDsKey, telegramID)
		} else {
			pipeline.HSet(ctx, userKey, models.UserFieldIsAdmin, adminMarkUnset)
			pipeline.SRem(ctx, models.AdminIDsKey, telegramID)
		}
		return nil
	})
	return err
}

func (repository *UserRepository) ListAdmins(ctx context.Context) ([]models.User, error) {
	adminIDs, err := repository.redisClient.SMembers(ctx, models.AdminIDsKey).Result()
	if err != nil {
		return nil, err
	}

	profileCommands := make([]*redis.MapStringStringCmd, 0, len(adminIDs))
	_, err = repository.redisClient.Pipelined(ctx, func(pipeline redis.Pipeliner) error {
		for _, storedAdminID := range adminIDs {
			adminID, err := strconv.ParseInt(storedAdminID, 10, 64)
			if err != nil {
				return err
			}
			profileCommands = append(profileCommands, pipeline.HGetAll(ctx, models.UserKey(adminID)))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	admins := make([]models.User, 0, len(profileCommands))
	for _, profileCommand := range profileCommands {
		var adminProfile models.User
		if err := profileCommand.Scan(&adminProfile); err != nil {
			return nil, err
		}
		admins = append(admins, adminProfile)
	}
	sort.Slice(admins, func(leftIndex, rightIndex int) bool {
		return admins[leftIndex].TelegramID < admins[rightIndex].TelegramID
	})
	return admins, nil
}
