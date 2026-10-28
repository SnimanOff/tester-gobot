package launcher

import (
	"archive/zip"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SnimanOff/testbot-platform/bot/children/archive"
	"github.com/SnimanOff/testbot-platform/database/models"
	"github.com/SnimanOff/testbot-platform/database/repositories"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

const testChildToken = "test-token"

const runningBotSource = `package main

import (
	"fmt"
	"os"
	"time"
)

func main() {
	if os.Getenv("BOT_TOKEN") != "test-token" {
		fmt.Println("no test token")
		os.Exit(3)
	}
	if os.Getenv("TESTER_BOT_TOKEN") != "" {
		fmt.Println("tester token leaked to child")
		os.Exit(4)
	}
	fmt.Println("bot is running")
	time.Sleep(time.Hour)
}
`

const crashingBotSource = `package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Println("invalid token")
	os.Exit(2)
}
`

const brokenBotSource = `package main

func main() {
	undefinedFunction()
}
`

type launcherFixture struct {
	launcher        *Launcher
	childRepository *repositories.ChildBotRepository
	childDatabase   *redis.Client
}

func newLauncherFixture(test *testing.T) *launcherFixture {
	test.Helper()
	test.Setenv("TESTER_BOT_TOKEN", "secret-token")

	fakeRedis := miniredis.RunT(test)
	mainDatabase := redis.NewClient(&redis.Options{Addr: fakeRedis.Addr(), DB: 0})
	childDatabase := redis.NewClient(&redis.Options{Addr: fakeRedis.Addr(), DB: 1})
	test.Cleanup(func() {
		mainDatabase.Close()
		childDatabase.Close()
	})

	childRepository := repositories.NewChildBotRepository(mainDatabase)
	testedLauncher, err := New(Options{
		ChildrenDirectory: filepath.Join(test.TempDir(), "children"),
		ChildBotToken:     testChildToken,
		ChildRedis:        ChildRedis{Address: fakeRedis.Addr(), Database: 1},
		ChildUser:         ChildUser{UID: -1, GID: -1},
		ArchiveLimits: archive.Limits{
			MaxArchiveBytes:     1024 * 1024,
			MaxUnpackedBytes:    1024 * 1024,
			MaxFiles:            10,
			MaxCompressionRatio: 100,
		},
		BuildTimeout: 2 * time.Minute,
		StartupCheck: 2 * time.Second,
	}, childRepository, childDatabase, nil)
	if err != nil {
		test.Fatal(err)
	}
	test.Cleanup(testedLauncher.StopActive)
	return &launcherFixture{launcher: testedLauncher, childRepository: childRepository, childDatabase: childDatabase}
}

func writeBotArchive(test *testing.T, archiveFiles map[string]string) string {
	test.Helper()
	archivePath := filepath.Join(test.TempDir(), "bot.zip")
	archiveFile, err := os.Create(archivePath)
	if err != nil {
		test.Fatal(err)
	}
	zipWriter := zip.NewWriter(archiveFile)
	for fileName, fileContent := range archiveFiles {
		entryWriter, err := zipWriter.Create(fileName)
		if err != nil {
			test.Fatal(err)
		}
		if _, err := entryWriter.Write([]byte(fileContent)); err != nil {
			test.Fatal(err)
		}
	}
	if err := zipWriter.Close(); err != nil {
		test.Fatal(err)
	}
	if err := archiveFile.Close(); err != nil {
		test.Fatal(err)
	}
	return archivePath
}

func (fixture *launcherFixture) requireStatus(test *testing.T, ownerID int64, expectedStatus string) models.ChildBot {
	test.Helper()
	childBot, err := fixture.childRepository.GetByOwnerID(context.Background(), ownerID)
	if err != nil || childBot.Status != expectedStatus {
		test.Fatalf("expected status %q, got %+v (err=%v)", expectedStatus, childBot, err)
	}
	return childBot
}

func TestRunningBotIsStartedAndStopped(test *testing.T) {
	fixture := newLauncherFixture(test)
	ctx := context.Background()
	const ownerID int64 = 7

	fixture.childDatabase.Set(ctx, "stale_key", "from previous run", 0)
	archivePath := writeBotArchive(test, map[string]string{
		"main.go": runningBotSource,
		"go.mod":  "module childbot\n\ngo 1.22\n",
	})
	if err := fixture.launcher.Deploy(ctx, ownerID, archivePath, "bot.zip"); err != nil {
		test.Fatalf("working bot did not start: %v", describeTestError(err))
	}
	fixture.requireStatus(test, ownerID, models.ChildStatusRunning)
	if keyCount := fixture.childDatabase.DBSize(ctx).Val(); keyCount != 0 {
		test.Fatalf("child database not flushed: %d keys", keyCount)
	}

	if activeOwnerID, isRunning := fixture.launcher.ActiveOwner(); !isRunning || activeOwnerID != ownerID {
		test.Fatalf("active bot: owner=%d running=%v", activeOwnerID, isRunning)
	}
	if fixture.launcher.StopOwned(ownerID + 1) {
		test.Fatal("other admin stopped the bot")
	}
	if !fixture.launcher.StopOwned(ownerID) {
		test.Fatal("owner could not stop the bot")
	}
	fixture.requireStatus(test, ownerID, models.ChildStatusStopped)
}

func TestCrashingBotReportsError(test *testing.T) {
	fixture := newLauncherFixture(test)
	archivePath := writeBotArchive(test, map[string]string{"main.go": crashingBotSource})

	deployError := fixture.launcher.Deploy(context.Background(), 8, archivePath, "bot.zip")
	var launchError *LaunchError
	if !errors.As(deployError, &launchError) || launchError.Stage != "запуск" || !strings.Contains(launchError.Output, "invalid token") {
		test.Fatalf("expected launch error with bot output, got: %v", describeTestError(deployError))
	}
	test.Logf("user reply: %s\n%s", launchError.Error(), launchError.Output)
	fixture.requireStatus(test, 8, models.ChildStatusFailed)
	if _, isRunning := fixture.launcher.ActiveOwner(); isRunning {
		test.Fatal("crashed bot is still active")
	}
}

func TestBrokenBotReportsBuildError(test *testing.T) {
	fixture := newLauncherFixture(test)
	archivePath := writeBotArchive(test, map[string]string{"main.go": brokenBotSource})

	deployError := fixture.launcher.Deploy(context.Background(), 9, archivePath, "bot.zip")
	var launchError *LaunchError
	if !errors.As(deployError, &launchError) || launchError.Stage != "сборка" || !strings.Contains(launchError.Output, "undefinedFunction") {
		test.Fatalf("expected build error, got: %v", describeTestError(deployError))
	}
	test.Logf("user reply: %s\n%s", launchError.Error(), launchError.Output)
	fixture.requireStatus(test, 9, models.ChildStatusFailed)
}

func describeTestError(deployError error) string {
	var launchError *LaunchError
	if errors.As(deployError, &launchError) {
		return launchError.Error() + "\n" + launchError.Output
	}
	if deployError == nil {
		return "no error"
	}
	return deployError.Error()
}
