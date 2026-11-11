package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

var environmentName = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,39}$`)

func composeEnvironment(environ []string) []string {
	variables := map[string]bool{
		"TESTER_BOT_TOKEN":  true,
		"OWNER_TELEGRAM_ID": true,
		"REDIS_PASSWORD":    true,
	}
	filtered := make([]string, 0, len(environ))
	for _, entry := range environ {
		name, _, _ := strings.Cut(entry, "=")
		if !variables[strings.ToUpper(name)] {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

func composeArguments(action, name, envFile, composeFile string) ([]string, error) {
	if !environmentName.MatchString(name) {
		return nil, fmt.Errorf("invalid environment name: use lowercase letters, digits, hyphens and underscores")
	}
	arguments := []string{"compose", "--project-name", "testenv-" + name, "--env-file", envFile, "-f", composeFile}
	switch action {
	case "up":
		arguments = append(arguments, "up", "-d", "--build")
	case "stop":
		arguments = append(arguments, "stop")
	case "status":
		arguments = append(arguments, "ps", "-a")
	case "logs":
		arguments = append(arguments, "logs", "--tail", "100")
	case "down":
		arguments = append(arguments, "down")
	default:
		return nil, fmt.Errorf("unknown action %q", action)
	}
	return arguments, nil
}

func main() {
	if len(os.Args) != 4 {
		fmt.Fprintln(os.Stderr, "usage: environments <up|stop|status|logs|down> <name> <env-file>")
		os.Exit(2)
	}
	envFile, err := filepath.Abs(os.Args[3])
	if err == nil {
		_, err = os.Stat(envFile)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	composeFile, err := filepath.Abs("docker-compose.environment.yml")
	if err == nil {
		_, err = os.Stat(composeFile)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	arguments, err := composeArguments(os.Args[1], os.Args[2], envFile, composeFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	command := exec.Command("docker", arguments...)
	command.Env = composeEnvironment(os.Environ())
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
