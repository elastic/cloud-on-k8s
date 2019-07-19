// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package command

import (
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
func (c *Command) Execute(ctx context.Context) ([]byte, error) {
	cmd := exec.CommandContext(ctx, c.executable, c.args...)
	cmd.Dir = c.workDir
	cmd.Env = append(os.Environ(), c.env...)

	return cmd.CombinedOutput()
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
