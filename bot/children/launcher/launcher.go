package launcher

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/SnimanOff/testbot-platform/bot/children/archive"
	"github.com/SnimanOff/testbot-platform/database/models"
	"github.com/SnimanOff/testbot-platform/database/repositories"
	"github.com/redis/go-redis/v9"
)

type Options struct {
	ChildrenDirectory string
	ChildBotToken     string
	ChildRedis        ChildRedis
	ChildUser         ChildUser
	ArchiveLimits     archive.Limits
	BuildTimeout      time.Duration
	StartupCheck      time.Duration
}

type ExitNotifier func(ownerID int64, exitReport string)

const (
	reportedOutputBytes = 3000
	stopTimeout         = 10 * time.Second
	sourceFolderName    = "source"
	logFileName         = "child.log"
)

type LaunchError struct {
	Stage  string
	Reason string
	Output string
}

func (launchError *LaunchError) Error() string {
	return launchError.Stage + ": " + launchError.Reason
}

type runningChild struct {
	ownerID       int64
	archiveName   string
	process       *os.Process
	output        *outputTail
	exited        chan struct{}
	exitError     error
	stopRequested atomic.Bool
}

type Launcher struct {
	options              Options
	childRepository      *repositories.ChildBotRepository
	childDatabase        *redis.Client
	notifyUnexpectedExit ExitNotifier

	deployMutex sync.Mutex
	activeMutex sync.Mutex
	activeChild *runningChild
}

func New(options Options, childRepository *repositories.ChildBotRepository, childDatabase *redis.Client, notifyUnexpectedExit ExitNotifier) (*Launcher, error) {
	if err := checkChildUserSupported(options.ChildUser); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(options.ChildrenDirectory, 0o755); err != nil {
		return nil, err
	}
	return &Launcher{
		options:              options,
		childRepository:      childRepository,
		childDatabase:        childDatabase,
		notifyUnexpectedExit: notifyUnexpectedExit,
	}, nil
}

func (launcher *Launcher) Deploy(ctx context.Context, ownerID int64, archivePath, archiveName string) error {
	launcher.deployMutex.Lock()
	defer launcher.deployMutex.Unlock()

	saveStatus := func(childStatus, lastError string) {
		launcher.saveStatus(ownerID, archiveName, childStatus, lastError)
	}
	failDeploy := func(deployError error) error {
		saveStatus(models.ChildStatusFailed, deployError.Error())
		return deployError
	}

	saveStatus(models.ChildStatusChecking, "")
	if err := archive.Inspect(archivePath, launcher.options.ArchiveLimits); err != nil {
		return failDeploy(err)
	}

	launcher.StopActive()

	workspaceDirectory := filepath.Join(launcher.options.ChildrenDirectory, strconv.FormatInt(ownerID, 10))
	sourceDirectory := filepath.Join(workspaceDirectory, sourceFolderName)
	if err := os.RemoveAll(workspaceDirectory); err != nil {
		return failDeploy(fmt.Errorf("clean workspace: %w", err))
	}
	if err := archive.Extract(archivePath, sourceDirectory, launcher.options.ArchiveLimits); err != nil {
		os.RemoveAll(workspaceDirectory)
		return failDeploy(err)
	}
	if err := handOverDirectory(workspaceDirectory, launcher.options.ChildUser); err != nil {
		return failDeploy(fmt.Errorf("hand over workspace: %w", err))
	}

	if err := launcher.childDatabase.FlushDB(ctx).Err(); err != nil {
		return failDeploy(fmt.Errorf("flush child database: %w", err))
	}

	saveStatus(models.ChildStatusBuilding, "")
	binaryPath := filepath.Join(workspaceDirectory, childBinaryName())
	if err := launcher.build(ctx, sourceDirectory, binaryPath); err != nil {
		return failDeploy(err)
	}

	startedChild, err := launcher.start(ownerID, archiveName, workspaceDirectory, sourceDirectory, binaryPath)
	if err != nil {
		return failDeploy(&LaunchError{Stage: "запуск", Reason: err.Error()})
	}

	startupTimer := time.NewTimer(launcher.options.StartupCheck)
	defer startupTimer.Stop()
	select {
	case <-startedChild.exited:
		launcher.forgetActive(startedChild)
		if startedChild.stopRequested.Load() {
			return failDeploy(&LaunchError{Stage: "запуск", Reason: "бот остановлен во время проверки запуска"})
		}
		return failDeploy(&LaunchError{
			Stage:  "запуск",
			Reason: "бот завершился сразу после запуска (" + describeExit(startedChild.exitError) + ")",
			Output: startedChild.output.String(),
		})
	case <-ctx.Done():
		launcher.StopActive()
		return failDeploy(ctx.Err())
	case <-startupTimer.C:
	}

	saveStatus(models.ChildStatusRunning, "")
	go launcher.watchExit(startedChild)
	return nil
}

func (launcher *Launcher) build(ctx context.Context, sourceDirectory, binaryPath string) error {
	buildContext, cancelBuild := context.WithTimeout(ctx, launcher.options.BuildTimeout)
	defer cancelBuild()

	_, statError := os.Stat(filepath.Join(sourceDirectory, "go.mod"))
	hasModuleFile := statError == nil
	buildArguments := []string{"build", "-o", binaryPath}
	if hasModuleFile {
		buildArguments = append(buildArguments, ".")
	} else {
		buildArguments = append(buildArguments, archive.EntryFileName)
	}

	buildOutput := newOutputTail(reportedOutputBytes)
	buildCommand := exec.CommandContext(buildContext, "go", buildArguments...)
	buildCommand.Dir = sourceDirectory
	buildCommand.Env = buildEnvironment(hasModuleFile)
	buildCommand.Stdout = buildOutput
	buildCommand.Stderr = buildOutput
	buildCommand.WaitDelay = stopTimeout
	prepareProcess(buildCommand, launcher.options.ChildUser)
	buildCommand.Cancel = func() error {
		return killProcess(buildCommand.Process)
	}

	if err := buildCommand.Run(); err != nil {
		buildReason := "go build завершился с ошибкой"
		if errors.Is(buildContext.Err(), context.DeadlineExceeded) {
			buildReason = fmt.Sprintf("сборка не уложилась в %s", launcher.options.BuildTimeout)
		}
		return &LaunchError{Stage: "сборка", Reason: buildReason, Output: buildOutput.String()}
	}
	return nil
}

func (launcher *Launcher) start(ownerID int64, archiveName, workspaceDirectory, sourceDirectory, binaryPath string) (*runningChild, error) {
	logFile, err := os.Create(filepath.Join(workspaceDirectory, logFileName))
	if err != nil {
		return nil, err
	}

	childOutput := newOutputTail(reportedOutputBytes)
	childCommand := exec.Command(binaryPath)
	childCommand.Dir = sourceDirectory
	childCommand.Env = childEnvironment(launcher.options.ChildBotToken, launcher.options.ChildRedis)
	childCommand.Stdout = io.MultiWriter(childOutput, logFile)
	childCommand.Stderr = childCommand.Stdout
	childCommand.WaitDelay = stopTimeout
	prepareProcess(childCommand, launcher.options.ChildUser)

	if err := childCommand.Start(); err != nil {
		logFile.Close()
		return nil, err
	}

	startedChild := &runningChild{
		ownerID:     ownerID,
		archiveName: archiveName,
		process:     childCommand.Process,
		output:      childOutput,
		exited:      make(chan struct{}),
	}
	go func() {
		startedChild.exitError = childCommand.Wait()
		logFile.Close()
		close(startedChild.exited)
	}()

	launcher.activeMutex.Lock()
	launcher.activeChild = startedChild
	launcher.activeMutex.Unlock()
	return startedChild, nil
}

func (launcher *Launcher) watchExit(watchedChild *runningChild) {
	<-watchedChild.exited
	if watchedChild.stopRequested.Load() {
		return
	}
	launcher.forgetActive(watchedChild)

	exitReport := "бот завершился (" + describeExit(watchedChild.exitError) + ")"
	launcher.saveStatus(watchedChild.ownerID, watchedChild.archiveName, models.ChildStatusFailed, exitReport)
	if childOutput := watchedChild.output.String(); childOutput != "" {
		exitReport += "\n\n" + childOutput
	}
	if launcher.notifyUnexpectedExit != nil {
		launcher.notifyUnexpectedExit(watchedChild.ownerID, exitReport)
	}
}

func (launcher *Launcher) StopActive() {
	launcher.activeMutex.Lock()
	stoppedChild := launcher.activeChild
	launcher.activeChild = nil
	launcher.activeMutex.Unlock()

	if stoppedChild == nil {
		return
	}
	stopProcess(stoppedChild)
	launcher.saveStatus(stoppedChild.ownerID, stoppedChild.archiveName, models.ChildStatusStopped, "")
}

func (launcher *Launcher) StopOwned(ownerID int64) bool {
	activeOwnerID, isRunning := launcher.ActiveOwner()
	if !isRunning || activeOwnerID != ownerID {
		return false
	}
	launcher.StopActive()
	return true
}

func (launcher *Launcher) ActiveOwner() (ownerID int64, isRunning bool) {
	launcher.activeMutex.Lock()
	defer launcher.activeMutex.Unlock()
	if launcher.activeChild == nil {
		return 0, false
	}
	return launcher.activeChild.ownerID, true
}

func (launcher *Launcher) forgetActive(finishedChild *runningChild) {
	launcher.activeMutex.Lock()
	defer launcher.activeMutex.Unlock()
	if launcher.activeChild == finishedChild {
		launcher.activeChild = nil
	}
}

func (launcher *Launcher) saveStatus(ownerID int64, archiveName, childStatus, lastError string) {
	saveContext, cancelSave := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelSave()
	err := launcher.childRepository.Upsert(saveContext, models.ChildBot{
		OwnerID:     ownerID,
		ArchiveName: archiveName,
		Status:      childStatus,
		LastError:   lastError,
	})
	if err != nil {
		log.Printf("save child bot %d status: %v", ownerID, err)
	}
}

func stopProcess(stoppedChild *runningChild) {
	stoppedChild.stopRequested.Store(true)
	if err := terminateProcess(stoppedChild.process); err != nil {
		killProcess(stoppedChild.process)
	}
	stopTimer := time.NewTimer(stopTimeout)
	defer stopTimer.Stop()
	select {
	case <-stoppedChild.exited:
	case <-stopTimer.C:
		killProcess(stoppedChild.process)
		<-stoppedChild.exited
	}
}

func describeExit(exitError error) string {
	var processExitError *exec.ExitError
	if errors.As(exitError, &processExitError) {
		return "код выхода " + strconv.Itoa(processExitError.ExitCode())
	}
	if exitError != nil {
		return exitError.Error()
	}
	return "код выхода 0"
}

func childBinaryName() string {
	if runtime.GOOS == "windows" {
		return "childbot.exe"
	}
	return "childbot"
}
