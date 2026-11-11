package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Settings struct {
	TesterBotToken  string
	ChildBotsLimit  int
	OwnerTelegramID int64

	RedisAddress  string
	RedisPassword string
	RedisDatabase int

	ChildRedisAddress  string
	ChildRedisPassword string
	ChildRedisDatabase int
	ChildRunUID        int
	ChildRunGID        int

	ChildContainerPrefix string
	ChildMemory          string
	ChildCPUs            string
	ChildPidsLimit       int

	ChildrenDirectory   string
	MaxArchiveBytes     int64
	MaxUnpackedBytes    int64
	MaxArchiveFiles     int
	MaxCompressionRatio int64
	ChildBuildTimeout   time.Duration
	ChildStartupCheck   time.Duration
}

const (
	bytesInMegabyte    = 1024 * 1024
	redisDatabaseCount = 16
)

var (
	ErrorSameRedisDatabase = errors.New("child bots need a separate database: set another CHILD_REDIS_ADDR or CHILD_REDIS_DB")
	ErrorTooManyChildren   = errors.New("CHILD_BOTS_LIMIT must be from 1 and CHILD_REDIS_DB plus CHILD_BOTS_LIMIT must not exceed 16")
)

func loadSettings() (*Settings, error) {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("loading .env: %w", err)
	}

	reader := &environmentReader{}
	loadedSettings := &Settings{
		TesterBotToken:  reader.requiredString("TESTER_BOT_TOKEN"),
		ChildBotsLimit:  int(reader.optionalInt64("CHILD_BOTS_LIMIT", 3)),
		OwnerTelegramID: reader.requiredInt64("OWNER_TELEGRAM_ID"),

		RedisAddress:  reader.optionalString("REDIS_ADDR", "localhost:6379"),
		RedisPassword: reader.optionalString("REDIS_PASSWORD", ""),
		RedisDatabase: int(reader.optionalInt64("REDIS_DB", 0)),

		ChildRedisAddress:  reader.optionalString("CHILD_REDIS_ADDR", "localhost:6380"),
		ChildRedisPassword: reader.optionalString("CHILD_REDIS_PASSWORD", ""),
		ChildRedisDatabase: int(reader.optionalInt64("CHILD_REDIS_DB", 0)),
		ChildRunUID:        int(reader.optionalInt64("CHILD_RUN_UID", -1)),
		ChildRunGID:        int(reader.optionalInt64("CHILD_RUN_GID", -1)),

		ChildContainerPrefix: reader.optionalString("CHILD_CONTAINER_PREFIX", ""),
		ChildMemory:          reader.optionalString("CHILD_MEMORY", "128m"),
		ChildCPUs:            reader.optionalString("CHILD_CPUS", "0.5"),
		ChildPidsLimit:       int(reader.optionalInt64("CHILD_PIDS_LIMIT", 64)),

		ChildrenDirectory:   reader.optionalString("CHILDREN_DIR", "./data/children"),
		MaxArchiveBytes:     reader.optionalInt64("MAX_ARCHIVE_SIZE_MB", 20) * bytesInMegabyte,
		MaxUnpackedBytes:    reader.optionalInt64("MAX_UNPACKED_SIZE_MB", 100) * bytesInMegabyte,
		MaxArchiveFiles:     int(reader.optionalInt64("MAX_ARCHIVE_FILES", 1000)),
		MaxCompressionRatio: reader.optionalInt64("MAX_COMPRESSION_RATIO", 100),
		ChildBuildTimeout:   time.Duration(reader.optionalInt64("CHILD_BUILD_TIMEOUT_SECONDS", 180)) * time.Second,
		ChildStartupCheck:   time.Duration(reader.optionalInt64("CHILD_STARTUP_CHECK_SECONDS", 10)) * time.Second,
	}

	if len(reader.problems) > 0 {
		return nil, fmt.Errorf("invalid settings: %s", strings.Join(reader.problems, "; "))
	}
	lastChildDatabase := loadedSettings.ChildRedisDatabase + loadedSettings.ChildBotsLimit - 1
	if loadedSettings.ChildBotsLimit < 1 || lastChildDatabase >= redisDatabaseCount {
		return nil, ErrorTooManyChildren
	}
	if loadedSettings.RedisAddress == loadedSettings.ChildRedisAddress &&
		loadedSettings.RedisDatabase >= loadedSettings.ChildRedisDatabase &&
		loadedSettings.RedisDatabase <= lastChildDatabase {
		return nil, ErrorSameRedisDatabase
	}
	return loadedSettings, nil
}

type environmentReader struct {
	problems []string
}

func (reader *environmentReader) requiredString(variableName string) string {
	variableValue := strings.TrimSpace(os.Getenv(variableName))
	if variableValue == "" {
		reader.problems = append(reader.problems, variableName+" environment variable not set")
	}
	return variableValue
}

func (reader *environmentReader) optionalString(variableName, defaultValue string) string {
	variableValue := strings.TrimSpace(os.Getenv(variableName))
	if variableValue == "" {
		return defaultValue
	}
	return variableValue
}

func (reader *environmentReader) requiredInt64(variableName string) int64 {
	variableValue := reader.requiredString(variableName)
	if variableValue == "" {
		return 0
	}
	return reader.parseInt64(variableName, variableValue)
}

func (reader *environmentReader) optionalInt64(variableName string, defaultValue int64) int64 {
	variableValue := strings.TrimSpace(os.Getenv(variableName))
	if variableValue == "" {
		return defaultValue
	}
	return reader.parseInt64(variableName, variableValue)
}

func (reader *environmentReader) parseInt64(variableName, variableValue string) int64 {
	parsedNumber, err := strconv.ParseInt(variableValue, 10, 64)
	if err != nil {
		reader.problems = append(reader.problems, variableName+" must be an integer")
		return 0
	}
	return parsedNumber
}
