// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package command

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Command is a CLI command to execute.
type Command struct {
	executable string
	workDir    string
	env        []string
	args       []string
}

// Execute runs the command and returns the output.
func (c *Command) Execute(ctx context.Context) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, c.executable, c.args...) //nolint:gosec
	cmd.Dir = c.workDir
	cmd.Env = append(os.Environ(), c.env...)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	stdout, err := cmd.Output()
	if err != nil {
		// We add stderr to the original error for convenience.
		err = fmt.Errorf("%w, stderr: %s", err, stderr.String())
	}
	return stdout, stderr.Bytes(), err
}

func (c *Command) String() string {
	return fmt.Sprintf("%s %s", c.executable, strings.Join(c.args, " "))
}

// Decorator allows optional modifications to a Command.
// See https://preslav.me/2019/07/07/implementing-a-functional-style-builder-in-go/ for details about the pattern.
type Decorator func(*Command) *Command

// New creates a command with the given arguments.
// Call Build() on the returned value to obtain the final command.
func New(executable string, args ...string) Decorator {
	return func(cmd *Command) *Command {
		cmd.executable = executable
		cmd.args = args
		return cmd
	}
}

// WithEnv sets the environment variables to use with this command.
// Each variable must be defined in the form k=v.
func (cd Decorator) WithEnv(env ...string) Decorator {
	return func(cmd *Command) *Command {
		cd(cmd).env = env
		return cmd
	}
}

// WithWorkDir sets the working directory of the command.
func (cd Decorator) WithWorkDir(dir string) Decorator {
	return func(cmd *Command) *Command {
		cd(cmd).workDir = dir
		return cmd
	}
}

// Build builds the final command with all the decorators applied.
func (cd Decorator) Build() *Command {
	return cd(&Command{})
}
