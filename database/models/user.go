package models

import (
	"strconv"
	"strings"
)

type User struct {
	TelegramID   int64  `redis:"telegram_id"`
	Username     string `redis:"username"`
	FirstName    string `redis:"first_name"`
	IsAdmin      bool   `redis:"is_admin"`
	RegisteredAt int64  `redis:"registered_at"`
}

const (
	UserFieldTelegramID   = "telegram_id"
	UserFieldUsername     = "username"
	UserFieldFirstName    = "first_name"
	UserFieldIsAdmin      = "is_admin"
	UserFieldRegisteredAt = "registered_at"
)

const AdminIDsKey = "admin_ids"

func UserKey(telegramID int64) string {
	return "user:" + strconv.FormatInt(telegramID, 10)
}

func UsernameKey(username string) string {
	return "username:" + strings.ToLower(strings.TrimPrefix(username, "@"))
}
