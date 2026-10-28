//go:build linux

package launcher

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

func checkChildUserSupported(childUser ChildUser) error {
	return nil
}

func prepareProcess(command *exec.Cmd, childUser ChildUser) {
	processAttributes := &syscall.SysProcAttr{Setpgid: true}
	if childUser.Enabled() {
		processAttributes.Credential = &syscall.Credential{
			Uid: uint32(childUser.UID),
			Gid: uint32(childUser.groupID()),
		}
	}
	command.SysProcAttr = processAttributes
}

func terminateProcess(process *os.Process) error {
	return syscall.Kill(-process.Pid, syscall.SIGTERM)
}

func killProcess(process *os.Process) error {
	return syscall.Kill(-process.Pid, syscall.SIGKILL)
}

func handOverDirectory(directoryPath string, childUser ChildUser) error {
	if !childUser.Enabled() {
		return nil
	}
	return filepath.WalkDir(directoryPath, func(entryPath string, directoryEntry fs.DirEntry, walkError error) error {
		if walkError != nil {
			return walkError
		}
		return os.Lchown(entryPath, childUser.UID, childUser.groupID())
	})
}
