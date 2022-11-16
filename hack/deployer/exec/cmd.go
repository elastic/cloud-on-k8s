// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package exec

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"text/template"
	"time"
)

// Command allows building commands to execute using fluent-style api
type Command struct {
	command      string
	context      context.Context
	logPrefix    string
	params       map[string]interface{}
	variablesSrc string
	variables    []string
	stream       bool
	stderr       bool
}

func NewCommand(command string) *Command {
	return &Command{command: command, stream: true, stderr: true}
}

func (c *Command) AsTemplate(params map[string]interface{}) *Command {
	c.params = params
	return c
}

func (c *Command) WithVariable(name, value string) *Command {
	c.variables = append(c.variables, name+"="+value)
	return c
}

func (c *Command) WithVariablesFromFile(filename string) *Command {
	c.variablesSrc = filename
	return c
}

func (c *Command) WithContext(ctx context.Context) *Command {
	c.context = ctx
	return c
}

func (c *Command) WithLog(logPrefix string) *Command {
	c.logPrefix = logPrefix
	return c
}

func (c *Command) WithoutStreaming() *Command {
	c.stream = false
	return c
}

func (c *Command) StdoutOnly() *Command {
	c.stderr = false
	return c
}

func (c *Command) Run() error {
	_, err := c.output()
	return err
}

func (c *Command) RunWithRetries(numAttempts int, timeout time.Duration) error {
	var err error
	for i := 0; i < numAttempts; i++ {
		ctx, cancelFunc := context.WithTimeout(context.Background(), timeout)
		err = c.WithContext(ctx).Run()
		cancelFunc()
		if err == nil {
			return nil
		}
	}
	return err
}

func (c *Command) Output() (string, error) {
	return c.output()
}

func (c *Command) OutputContainsAny(tokens ...string) (bool, error) {
	out, err := c.output()

	for _, token := range tokens {
		if strings.Contains(out, token) {
			return true, err
		}
	}

	if err != nil {
		// provide additional context to callers otherwise it is really hard to figure out what went wrong
		err = fmt.Errorf("%s with err: %w", out, err)
	}
	return false, err
}

func (c *Command) OutputList() (list []string, err error) {
	out, err := c.output()
	if err != nil {
		return nil, err
	}

	for _, item := range strings.Split(out, "\n") {
		if item != "" {
			list = append(list, item)
		}
	}

	return
}

func (c *Command) output() (string, error) {
	if c.params != nil {
		var b bytes.Buffer
		if err := template.Must(template.New("").
			Funcs(template.FuncMap{"Join": strings.Join}).
			Parse(c.command)).Execute(&b, c.params); err != nil {
			return "", err
		}
		c.command = b.String()
	}

	if c.logPrefix != "" {
		log.Printf("%s: %s", c.logPrefix, c.command)
	}

	var cmd *exec.Cmd
	if c.context != nil {
		cmd = exec.CommandContext(c.context, "/usr/bin/env", "bash", "-c", c.command) // #nosec G204
	} else {
		cmd = exec.Command("/usr/bin/env", "bash", "-c", c.command) // #nosec G204
	}

	// support .env or similar files to specify environment variables
	if c.variablesSrc != "" {
		bytes, err := os.ReadFile(c.variablesSrc)
		if err != nil {
			return "", err
		}
		// assume k=v pair lines
		c.variables = append(c.variables, strings.Split(string(bytes), "\n")...)
	}

	cmd.Env = append(os.Environ(), c.variables...)

	b := bytes.Buffer{}
	if c.stream {
		cmd.Stdout = io.MultiWriter(os.Stdout, &b)
		cmd.Stderr = io.MultiWriter(os.Stderr, &b)
	} else {
		cmd.Stdout = &b
		cmd.Stderr = &b
	}

	if !c.stderr {
		cmd.Stderr = nil
	}

	err := cmd.Run()
	return b.String(), err
}
