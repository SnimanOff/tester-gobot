package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Settings struct {
	TesterBotToken  string
	OwnerTelegramID int64
}

func loadSettings() (*Settings, error) {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("loading .env: %w", err)
	}

	reader := &environmentReader{}
	loadedSettings := &Settings{
		TesterBotToken:  reader.requiredString("TESTER_BOT_TOKEN"),
		OwnerTelegramID: reader.requiredInt64("OWNER_TELEGRAM_ID"),
	}

	if len(reader.problems) > 0 {
		return nil, fmt.Errorf("invalid settings: %s", strings.Join(reader.problems, "; "))
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

func (reader *environmentReader) requiredInt64(variableName string) int64 {
	variableValue := reader.requiredString(variableName)
	if variableValue == "" {
		return 0
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
