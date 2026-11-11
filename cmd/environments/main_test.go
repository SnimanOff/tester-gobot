package main

import (
	"reflect"
	"testing"
)

func TestComposeArguments(t *testing.T) {
	arguments, err := composeArguments("up", "alpha", "/tmp/alpha.env", "/app/docker-compose.environment.yml")
	if err != nil {
		t.Fatal(err)
	}
	expected := []string{"compose", "--project-name", "testenv-alpha", "--env-file", "/tmp/alpha.env", "-f", "/app/docker-compose.environment.yml", "up", "-d", "--build"}
	if !reflect.DeepEqual(arguments, expected) {
		t.Fatalf("expected %v, got %v", expected, arguments)
	}
}

func TestInvalidEnvironmentName(t *testing.T) {
	for _, name := range []string{"", "../other", "-name", "UPPER", "name with spaces"} {
		if _, err := composeArguments("up", name, "env", "compose"); err == nil {
			t.Fatalf("accepted invalid name %q", name)
		}
	}
}

func TestDownKeepsVolumes(t *testing.T) {
	arguments, err := composeArguments("down", "alpha", "env", "compose")
	if err != nil {
		t.Fatal(err)
	}
	if arguments[len(arguments)-1] != "down" {
		t.Fatalf("unexpected arguments: %v", arguments)
	}
}

func TestComposeUsesEnvironmentFileValues(t *testing.T) {
	filtered := composeEnvironment([]string{"PATH=value", "tester_bot_token=host", "REDIS_PASSWORD=host", "OTHER=value"})
	expected := []string{"PATH=value", "OTHER=value"}
	if !reflect.DeepEqual(filtered, expected) {
		t.Fatalf("expected %v, got %v", expected, filtered)
	}
}
