package launcher

import (
	"slices"
	"strings"
	"testing"
)

func TestContainerArguments(test *testing.T) {
	container := Container{
		Prefix:    "testenv-alpha",
		Image:     "sha256:image",
		Memory:    "128m",
		CPUs:      "0.5",
		PidsLimit: 64,
		User:      ChildUser{UID: 10001, GID: 10001},
	}
	arguments := container.createArguments(2, childEnvironment("secret-token", ChildRedis{Address: "child-redis:6379", Database: 1}))
	joined := strings.Join(arguments, " ")

	for _, expected := range []string{
		"--name testenv-alpha-child-2",
		"--network testenv-alpha_children",
		"--memory 128m",
		"--cpus 0.5",
		"--pids-limit 64",
		"--cap-drop ALL",
		"--security-opt no-new-privileges",
		"--user 10001:10001",
		"--env BOT_TOKEN",
		"--env REDIS_DB",
		"sha256:image /child/childbot",
	} {
		if !strings.Contains(joined, expected) {
			test.Fatalf("missing %q in %v", expected, arguments)
		}
	}
	if strings.Contains(joined, "secret-token") {
		test.Fatal("token value passed in arguments")
	}
	if !slices.Contains(arguments, "--rm") {
		test.Fatal("container is not removed after exit")
	}
}
