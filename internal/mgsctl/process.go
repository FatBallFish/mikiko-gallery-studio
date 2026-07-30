package mgsctl

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
)

type OSProcessRunner struct {
	Stdout io.Writer
	Stderr io.Writer
}

func (runner OSProcessRunner) Run(ctx context.Context, spec ProcessSpec) error {
	command := exec.CommandContext(ctx, spec.Executable, spec.Arguments...)
	command.Dir = spec.Directory
	command.Env = spec.Environment
	command.Stdout = runner.Stdout
	command.Stderr = runner.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("process exited unsuccessfully: %w", err)
	}
	return nil
}

func (spec ProcessSpec) String() string {
	return fmt.Sprintf("ProcessSpec{Executable:%q, Arguments:%q, Directory:%q, Environment:<redacted>}", spec.Executable, spec.Arguments, spec.Directory)
}

func (spec ProcessSpec) GoString() string { return spec.String() }

func (spec ProcessSpec) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Executable  string   `json:"executable"`
		Arguments   []string `json:"arguments"`
		Directory   string   `json:"directory"`
		Environment string   `json:"environment"`
	}{Executable: spec.Executable, Arguments: spec.Arguments, Directory: spec.Directory, Environment: "REDACTED"})
}
