package models

import "strconv"

type ManagedBot struct {
	Number   int    `redis:"number"`
	BotID    int64  `redis:"bot_id"`
	Username string `redis:"username"`
	OwnerID  int64  `redis:"owner_id"`
	Token    string `redis:"token"`
}

const ManagedBotNumbersKey = "managed_bot_numbers"

func ManagedBotKey(number int) string {
	return "managed_bot:" + strconv.Itoa(number)
}
