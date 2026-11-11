package models

import "strconv"

type ChildBot struct {
	Slot        int    `redis:"slot"`
	OwnerID     int64  `redis:"owner_id"`
	ArchiveName string `redis:"archive_name"`
	Status      string `redis:"status"`
	LastError   string `redis:"last_error"`
	UpdatedAt   int64  `redis:"updated_at"`
}

const (
	ChildFieldStatus    = "status"
	ChildFieldLastError = "last_error"
	ChildFieldUpdatedAt = "updated_at"
)

const (
	ChildStatusChecking = "checking"
	ChildStatusBuilding = "building"
	ChildStatusRunning  = "running"
	ChildStatusFailed   = "failed"
	ChildStatusStopped  = "stopped"
)

const ChildBotKeyPattern = "child_bot:*"

func ChildBotKey(slot int) string {
	return "child_bot:" + strconv.Itoa(slot)
}
