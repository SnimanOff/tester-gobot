//go:build docker

package launcher

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/SnimanOff/testbot-platform/bot/children/archive"
	"github.com/SnimanOff/testbot-platform/database/models"
	"github.com/SnimanOff/testbot-platform/database/repositories"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

const dockerTestPrefix = "kursovaya-check"

func dockerOutput(test *testing.T, arguments ...string) string {
	test.Helper()
	output, err := exec.Command("docker", arguments...).CombinedOutput()
	if err != nil {
		test.Fatalf("docker %v: %v\n%s", arguments, err, output)
	}
	return strings.TrimSpace(string(output))
}

func TestBotsRunInContainers(test *testing.T) {
	image := os.Getenv("TESTBOT_CHILD_IMAGE")
	if image == "" {
		image = "alpine:3.20"
	}
	exec.Command("docker", "network", "rm", dockerTestPrefix+"_children").Run()
	dockerOutput(test, "network", "create", dockerTestPrefix+"_children")
	test.Cleanup(func() {
		exec.Command("docker", "network", "rm", dockerTestPrefix+"_children").Run()
	})

	fakeRedis := miniredis.RunT(test)
	mainDatabase := redis.NewClient(&redis.Options{Addr: fakeRedis.Addr()})
	test.Cleanup(func() { mainDatabase.Close() })
	childRepository := repositories.NewChildBotRepository(mainDatabase)

	testedLauncher, err := New(Options{
		ChildrenDirectory: filepath.Join(test.TempDir(), "children"),
		MaxChildren:       3,
		ChildRedis:        ChildRedis{Address: fakeRedis.Addr(), Database: 1},
		ChildUser:         ChildUser{UID: -1, GID: -1},
		Container: Container{
			Prefix:    dockerTestPrefix,
			Image:     image,
			Memory:    "128m",
			CPUs:      "0.5",
			PidsLimit: 64,
			User:      ChildUser{UID: 10001, GID: 10001},
		},
		ArchiveLimits: archive.Limits{
			MaxArchiveBytes:     1024 * 1024,
			MaxUnpackedBytes:    1024 * 1024,
			MaxFiles:            10,
			MaxCompressionRatio: 100,
		},
		BuildTimeout: 2 * time.Minute,
		StartupCheck: 3 * time.Second,
	}, childRepository, nil)
	if err != nil {
		test.Fatal(err)
	}
	test.Cleanup(testedLauncher.StopAll)

	archivePath := writeBotArchive(test, map[string]string{"main.go": runningBotSource})
	for slot := 1; slot <= 3; slot++ {
		if err := testedLauncher.Deploy(context.Background(), 7, slot, "test-token-"+strconv.Itoa(slot), archivePath, "bot.zip"); err != nil {
			test.Fatalf("bot %d did not start: %v", slot, describeTestError(err))
		}
	}

	runningNames := dockerOutput(test, "ps", "--filter", "label="+containerLabel+"="+dockerTestPrefix, "--format", "{{.Names}}")
	test.Logf("running child containers:\n%s", runningNames)
	for slot := 1; slot <= 3; slot++ {
		containerName := dockerTestPrefix + "-child-" + strconv.Itoa(slot)
		if !strings.Contains(runningNames, containerName) {
			test.Fatalf("container %s is not running", containerName)
		}
		limits := dockerOutput(test, "inspect", "--format", "{{.HostConfig.Memory}} {{.HostConfig.NanoCpus}} {{.HostConfig.PidsLimit}} {{.Config.User}} {{.HostConfig.CapDrop}}", containerName)
		test.Logf("%s limits: %s", containerName, limits)
		if limits != "134217728 500000000 64 10001:10001 [ALL]" {
			test.Fatalf("unexpected limits for %s: %s", containerName, limits)
		}
		childBot, err := childRepository.GetBySlot(context.Background(), slot)
		if err != nil || childBot.Status != models.ChildStatusRunning {
			test.Fatalf("slot %d status: %+v %v", slot, childBot, err)
		}
	}

	if holdSeconds, _ := strconv.Atoi(os.Getenv("TESTBOT_HOLD_SECONDS")); holdSeconds > 0 {
		time.Sleep(time.Duration(holdSeconds) * time.Second)
	}

	testedLauncher.StopAll()
	if leftNames := dockerOutput(test, "ps", "--all", "--filter", "label="+containerLabel+"="+dockerTestPrefix, "--format", "{{.Names}}"); leftNames != "" {
		test.Fatalf("containers left after stop: %s", leftNames)
	}
}
