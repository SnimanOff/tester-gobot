package launcher

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type Container struct {
	Prefix    string
	Image     string
	Memory    string
	CPUs      string
	PidsLimit int
	User      ChildUser
}

const (
	containerLabel     = "testbot.children"
	containerWorkspace = "/child"
	containerBinary    = "childbot"
)

func (container Container) Enabled() bool {
	return container.Prefix != ""
}

func (container Container) Name(slot int) string {
	return container.Prefix + "-child-" + strconv.Itoa(slot)
}

func (container Container) Network() string {
	return container.Prefix + "_children"
}

func (container Container) createArguments(slot int, environment []string) []string {
	arguments := []string{"create", "--rm",
		"--name", container.Name(slot),
		"--label", containerLabel + "=" + container.Prefix,
		"--network", container.Network(),
		"--memory", container.Memory,
		"--cpus", container.CPUs,
		"--pids-limit", strconv.Itoa(container.PidsLimit),
		"--cap-drop", "ALL",
		"--security-opt", "no-new-privileges",
		"--workdir", containerWorkspace + "/" + sourceFolderName,
	}
	if container.User.Enabled() {
		arguments = append(arguments, "--user", fmt.Sprintf("%d:%d", container.User.UID, container.User.groupID()))
	}
	for _, environmentEntry := range environment {
		variableName, _, _ := strings.Cut(environmentEntry, "=")
		arguments = append(arguments, "--env", variableName)
	}
	return append(arguments, container.Image, containerWorkspace+"/"+containerBinary)
}

func (container Container) prepare(slot int, workspaceDirectory string, environment []string) error {
	removeContainers(container.Name(slot))

	createCommand := exec.Command("docker", container.createArguments(slot, environment)...)
	createCommand.Env = append(inheritedEnvironment(), environment...)
	if output, err := createCommand.CombinedOutput(); err != nil {
		return fmt.Errorf("docker create: %s", strings.TrimSpace(string(output)))
	}

	copyCommand := exec.Command("docker", "cp", "--archive", workspaceDirectory+string(filepath.Separator)+".", container.Name(slot)+":"+containerWorkspace)
	if output, err := copyCommand.CombinedOutput(); err != nil {
		removeContainers(container.Name(slot))
		return fmt.Errorf("docker cp: %s", strings.TrimSpace(string(output)))
	}
	return nil
}

func (container Container) removeLeftovers() {
	listOutput, err := exec.Command("docker", "ps", "--all", "--quiet", "--filter", "label="+containerLabel+"="+container.Prefix).Output()
	if err != nil {
		return
	}
	removeContainers(strings.Fields(string(listOutput))...)
}

func removeContainers(containerNames ...string) {
	if len(containerNames) == 0 {
		return
	}
	exec.Command("docker", append([]string{"rm", "--force"}, containerNames...)...).Run()
}

func currentImage() (string, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return "", err
	}
	inspectOutput, err := exec.Command("docker", "inspect", "--format", "{{.Image}}", hostname).Output()
	if err != nil {
		return "", fmt.Errorf("inspect own container %s: %w", hostname, err)
	}
	return strings.TrimSpace(string(inspectOutput)), nil
}
