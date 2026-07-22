package servicehost

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"time"
)

type ChildOptions struct {
	ServiceName      string
	WorkingDirectory string
	Executable       string
	Arguments        []string
	LogDirectory     string
	RestartExitCode  int
	RestartDelay     time.Duration
	Environment      []string
}

var serviceNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

func Validate(options ChildOptions) error {
	if !serviceNamePattern.MatchString(options.ServiceName) {
		return fmt.Errorf("service name must contain only letters, digits, dot, underscore, or hyphen")
	}
	for label, value := range map[string]string{
		"working directory": options.WorkingDirectory,
		"executable":        options.Executable,
		"log directory":     options.LogDirectory,
	} {
		if value == "" || !filepath.IsAbs(value) {
			return fmt.Errorf("%s must be an absolute path", label)
		}
	}
	if options.RestartExitCode < 1 || options.RestartExitCode > 255 {
		return fmt.Errorf("restart exit code must be between 1 and 255")
	}
	return nil
}

func RunChild(ctx context.Context, options ChildOptions) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := Validate(options); err != nil {
		return err
	}
	if options.RestartDelay <= 0 {
		options.RestartDelay = time.Second
	}
	if err := os.MkdirAll(options.LogDirectory, 0o700); err != nil {
		return fmt.Errorf("create service log directory: %w", err)
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil
		}
		exitCode, err := runOnce(ctx, options)
		if ctx.Err() != nil {
			return nil
		}
		if err != nil && exitCode < 0 {
			return err
		}
		if exitCode != options.RestartExitCode {
			if err != nil {
				return fmt.Errorf("service child exited with exit code %d: %w", exitCode, err)
			}
			return fmt.Errorf("service child exited unexpectedly with exit code %d", exitCode)
		}
		timer := time.NewTimer(options.RestartDelay)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return nil
		case <-timer.C:
		}
	}
}

func runOnce(ctx context.Context, options ChildOptions) (int, error) {
	stdout, err := os.OpenFile(filepath.Join(options.LogDirectory, options.ServiceName+".out.log"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return -1, fmt.Errorf("open service stdout log: %w", err)
	}
	defer stdout.Close()
	stderr, err := os.OpenFile(filepath.Join(options.LogDirectory, options.ServiceName+".err.log"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return -1, fmt.Errorf("open service stderr log: %w", err)
	}
	defer stderr.Close()
	command := exec.CommandContext(ctx, options.Executable, options.Arguments...)
	command.Dir = options.WorkingDirectory
	command.Stdout = stdout
	command.Stderr = stderr
	if options.Environment != nil {
		command.Env = slices.Clone(options.Environment)
	}
	err = command.Run()
	if err == nil {
		return 0, nil
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		return exitError.ExitCode(), err
	}
	return -1, fmt.Errorf("start service child: %w", err)
}
