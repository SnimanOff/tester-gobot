//go:build !linux

package launcher

import (
	"errors"
	"os"
	"os/exec"
)

var ErrorChildUserUnsupported = errors.New("CHILD_RUN_UID is supported only on linux, set -1")

func checkChildUserSupported(childUser ChildUser) error {
	if childUser.Enabled() {
		return ErrorChildUserUnsupported
	}
	return nil
}

func prepareProcess(command *exec.Cmd, childUser ChildUser) {}

func terminateProcess(process *os.Process) error {
	return process.Kill()
}

func killProcess(process *os.Process) error {
	return process.Kill()
}

func handOverDirectory(directoryPath string, childUser ChildUser) error {
	return nil
}
