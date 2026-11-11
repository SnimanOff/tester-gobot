package commands

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/SnimanOff/testbot-platform/bot/children/launcher"
	telegram "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func StopChildCommand(childLauncher *launcher.Launcher, ownerTelegramID int64) telegram.HandlerFunc {
	return func(ctx context.Context, tgBot *telegram.Bot, update *models.Update) {
		if update.Message == nil || update.Message.From == nil {
			return
		}

		from := update.Message.From
		chatID := update.Message.Chat.ID

		slots := make([]int, 0, childLauncher.Slots())
		if _, slotText, hasSlot := strings.Cut(strings.TrimSpace(update.Message.Text), " "); hasSlot {
			slot, err := strconv.Atoi(strings.TrimSpace(slotText))
			if err != nil || slot < 1 || slot > childLauncher.Slots() {
				sendText(ctx, tgBot, chatID, fmt.Sprintf("Номер бота должен быть от 1 до %d.", childLauncher.Slots()))
				return
			}
			slots = append(slots, slot)
		} else {
			for slot := 1; slot <= childLauncher.Slots(); slot++ {
				slots = append(slots, slot)
			}
		}

		var stoppedSlots []string
		for _, slot := range slots {
			isStopped := false
			if from.ID == ownerTelegramID {
				isStopped = childLauncher.Stop(slot)
			} else {
				isStopped = childLauncher.StopOwned(from.ID, slot)
			}
			if isStopped {
				stoppedSlots = append(stoppedSlots, strconv.Itoa(slot))
			}
		}

		if len(stoppedSlots) == 0 {
			sendText(ctx, tgBot, chatID, "Нет запущенных ботов.")
			return
		}
		sendText(ctx, tgBot, chatID, "Остановлены боты: "+strings.Join(stoppedSlots, ", ")+".")
	}
}
