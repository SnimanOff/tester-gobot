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
	MaxChildren       int
	ChildRedis        ChildRedis
	ChildUser         ChildUser
	Container         Container
	ArchiveLimits     archive.Limits
	BuildTimeout      time.Duration
	StartupCheck      time.Duration
}

type ExitNotifier func(ownerID int64, slot int, exitReport string)

const (
	reportedOutputBytes = 3000
	stopTimeout         = 10 * time.Second
	sourceFolderName    = "source"
	logFileName         = "child.log"
)

var ErrorUnknownSlot = errors.New("unknown child bot slot")

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
	slot          int
	archiveName   string
	process       *os.Process
	containerName string
	output        *outputTail
	exited        chan struct{}
	exitError     error
	stopRequested atomic.Bool
}

type Launcher struct {
	options              Options
	childRepository      *repositories.ChildBotRepository
	notifyUnexpectedExit ExitNotifier

	deployMutex    sync.Mutex
	activeMutex    sync.Mutex
	activeChildren map[int]*runningChild
}

func New(options Options, childRepository *repositories.ChildBotRepository, notifyUnexpectedExit ExitNotifier) (*Launcher, error) {
	if options.MaxChildren < 1 {
		return nil, errors.New("no child bots allowed")
	}
	if !options.Container.Enabled() {
		if err := checkChildUserSupported(options.ChildUser); err != nil {
			return nil, err
		}
	}
	if err := os.MkdirAll(options.ChildrenDirectory, 0o755); err != nil {
		return nil, err
	}
	if options.Container.Enabled() {
		if options.Container.Image == "" {
			image, err := currentImage()
			if err != nil {
				return nil, err
			}
			options.Container.Image = image
		}
		options.Container.removeLeftovers()
	}
	return &Launcher{
		options:              options,
		childRepository:      childRepository,
		notifyUnexpectedExit: notifyUnexpectedExit,
		activeChildren:       map[int]*runningChild{},
	}, nil
}

func (launcher *Launcher) Slots() int {
	return launcher.options.MaxChildren
}

func (launcher *Launcher) Deploy(ctx context.Context, ownerID int64, slot int, botToken, archivePath, archiveName string) error {
	if slot < 1 || slot > launcher.Slots() {
		return ErrorUnknownSlot
	}
	launcher.deployMutex.Lock()
	defer launcher.deployMutex.Unlock()

	saveStatus := func(childStatus, lastError string) {
		launcher.saveStatus(slot, ownerID, archiveName, childStatus, lastError)
	}
	failDeploy := func(deployError error) error {
		saveStatus(models.ChildStatusFailed, deployError.Error())
		return deployError
	}

	saveStatus(models.ChildStatusChecking, "")
	if err := archive.Inspect(archivePath, launcher.options.ArchiveLimits); err != nil {
		return failDeploy(err)
	}

	launcher.Stop(slot)

	workspaceDirectory := filepath.Join(launcher.options.ChildrenDirectory, strconv.Itoa(slot))
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

	if err := launcher.flushChildDatabase(ctx, slot); err != nil {
		return failDeploy(fmt.Errorf("flush child database: %w", err))
	}

	saveStatus(models.ChildStatusBuilding, "")
	binaryPath := filepath.Join(workspaceDirectory, launcher.binaryName())
	if err := launcher.build(ctx, sourceDirectory, binaryPath); err != nil {
		return failDeploy(err)
	}

	startedChild, err := launcher.start(ownerID, slot, botToken, archiveName, workspaceDirectory, sourceDirectory, binaryPath)
	if err != nil {
		return failDeploy(&LaunchError{Stage: "запуск", Reason: err.Error()})
	}

	startupTimer := time.NewTimer(launcher.options.StartupCheck)
	defer startupTimer.Stop()
	select {
	case <-startedChild.exited:
		launcher.forget(startedChild)
		if startedChild.stopRequested.Load() {
			return failDeploy(&LaunchError{Stage: "запуск", Reason: "бот остановлен во время проверки запуска"})
		}
		return failDeploy(&LaunchError{
			Stage:  "запуск",
			Reason: "бот завершился сразу после запуска (" + describeExit(startedChild.exitError) + ")",
			Output: startedChild.output.String(),
		})
	case <-ctx.Done():
		launcher.Stop(slot)
		return failDeploy(ctx.Err())
	case <-startupTimer.C:
	}

	saveStatus(models.ChildStatusRunning, "")
	go launcher.watchExit(startedChild)
	return nil
}

func (launcher *Launcher) childRedis(slot int) ChildRedis {
	childRedis := launcher.options.ChildRedis
	childRedis.Database += slot - 1
	return childRedis
}

func (launcher *Launcher) flushChildDatabase(ctx context.Context, slot int) error {
	childRedis := launcher.childRedis(slot)
	childDatabase := redis.NewClient(&redis.Options{
		Addr:     childRedis.Address,
		Password: childRedis.Password,
		DB:       childRedis.Database,
	})
	defer childDatabase.Close()
	return childDatabase.FlushDB(ctx).Err()
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
	if launcher.options.Container.Enabled() {
		buildCommand.Env = append(buildCommand.Env, "GOOS=linux")
	}
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

func (launcher *Launcher) childCommand(slot int, botToken, workspaceDirectory, sourceDirectory, binaryPath string) (*exec.Cmd, string, error) {
	childVariables := childEnvironment(botToken, launcher.childRedis(slot))
	container := launcher.options.Container
	if !container.Enabled() {
		childCommand := exec.Command(binaryPath)
		childCommand.Dir = sourceDirectory
		childCommand.Env = append(inheritedEnvironment(), childVariables...)
		prepareProcess(childCommand, launcher.options.ChildUser)
		return childCommand, "", nil
	}

	if err := container.prepare(slot, workspaceDirectory, childVariables); err != nil {
		return nil, "", err
	}
	childCommand := exec.Command("docker", "start", "--attach", container.Name(slot))
	childCommand.Env = inheritedEnvironment()
	return childCommand, container.Name(slot), nil
}

func (launcher *Launcher) start(ownerID int64, slot int, botToken, archiveName, workspaceDirectory, sourceDirectory, binaryPath string) (*runningChild, error) {
	childCommand, containerName, err := launcher.childCommand(slot, botToken, workspaceDirectory, sourceDirectory, binaryPath)
	if err != nil {
		return nil, err
	}

	logFile, err := os.Create(filepath.Join(workspaceDirectory, logFileName))
	if err != nil {
		removeContainers(containerName)
		return nil, err
	}

	childOutput := newOutputTail(reportedOutputBytes)
	childCommand.Stdout = io.MultiWriter(childOutput, logFile)
	childCommand.Stderr = childCommand.Stdout
	childCommand.WaitDelay = stopTimeout

	if err := childCommand.Start(); err != nil {
		logFile.Close()
		removeContainers(containerName)
		return nil, err
	}

	startedChild := &runningChild{
		ownerID:       ownerID,
		slot:          slot,
		archiveName:   archiveName,
		process:       childCommand.Process,
		containerName: containerName,
		output:        childOutput,
		exited:        make(chan struct{}),
	}
	go func() {
		startedChild.exitError = childCommand.Wait()
		logFile.Close()
		close(startedChild.exited)
	}()

	launcher.activeMutex.Lock()
	launcher.activeChildren[slot] = startedChild
	launcher.activeMutex.Unlock()
	return startedChild, nil
}

func (launcher *Launcher) watchExit(watchedChild *runningChild) {
	<-watchedChild.exited
	if watchedChild.stopRequested.Load() {
		return
	}
	launcher.forget(watchedChild)

	exitReport := "бот завершился (" + describeExit(watchedChild.exitError) + ")"
	launcher.saveStatus(watchedChild.slot, watchedChild.ownerID, watchedChild.archiveName, models.ChildStatusFailed, exitReport)
	if childOutput := watchedChild.output.String(); childOutput != "" {
		exitReport += "\n\n" + childOutput
	}
	if launcher.notifyUnexpectedExit != nil {
		launcher.notifyUnexpectedExit(watchedChild.ownerID, watchedChild.slot, exitReport)
	}
}

func (launcher *Launcher) Stop(slot int) bool {
	launcher.activeMutex.Lock()
	stoppedChild := launcher.activeChildren[slot]
	delete(launcher.activeChildren, slot)
	launcher.activeMutex.Unlock()

	if stoppedChild == nil {
		return false
	}
	stopChild(stoppedChild)
	launcher.saveStatus(slot, stoppedChild.ownerID, stoppedChild.archiveName, models.ChildStatusStopped, "")
	return true
}

func (launcher *Launcher) StopOwned(ownerID int64, slot int) bool {
	activeOwnerID, isRunning := launcher.SlotOwner(slot)
	if !isRunning || activeOwnerID != ownerID {
		return false
	}
	return launcher.Stop(slot)
}

func (launcher *Launcher) StopAll() {
	for slot := 1; slot <= launcher.Slots(); slot++ {
		launcher.Stop(slot)
	}
}

func (launcher *Launcher) SlotOwner(slot int) (ownerID int64, isRunning bool) {
	launcher.activeMutex.Lock()
	defer launcher.activeMutex.Unlock()
	activeChild := launcher.activeChildren[slot]
	if activeChild == nil {
		return 0, false
	}
	return activeChild.ownerID, true
}

func (launcher *Launcher) forget(finishedChild *runningChild) {
	launcher.activeMutex.Lock()
	defer launcher.activeMutex.Unlock()
	if launcher.activeChildren[finishedChild.slot] == finishedChild {
		delete(launcher.activeChildren, finishedChild.slot)
	}
}

func (launcher *Launcher) saveStatus(slot int, ownerID int64, archiveName, childStatus, lastError string) {
	saveContext, cancelSave := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelSave()
	err := launcher.childRepository.Upsert(saveContext, models.ChildBot{
		Slot:        slot,
		OwnerID:     ownerID,
		ArchiveName: archiveName,
		Status:      childStatus,
		LastError:   lastError,
	})
	if err != nil {
		log.Printf("save child bot %d status: %v", slot, err)
	}
}

func (launcher *Launcher) binaryName() string {
	if launcher.options.Container.Enabled() {
		return containerBinary
	}
	if runtime.GOOS == "windows" {
		return "childbot.exe"
	}
	return "childbot"
}

func stopChild(stoppedChild *runningChild) {
	stoppedChild.stopRequested.Store(true)
	if stoppedChild.containerName != "" {
		exec.Command("docker", "stop", "--time", strconv.Itoa(int(stopTimeout.Seconds())), stoppedChild.containerName).Run()
	} else if err := terminateProcess(stoppedChild.process); err != nil {
		killProcess(stoppedChild.process)
	}
	stopTimer := time.NewTimer(stopTimeout)
	defer stopTimer.Stop()
	select {
	case <-stoppedChild.exited:
	case <-stopTimer.C:
		if stoppedChild.containerName != "" {
			removeContainers(stoppedChild.containerName)
			stoppedChild.process.Kill()
		} else {
			killProcess(stoppedChild.process)
		}
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
