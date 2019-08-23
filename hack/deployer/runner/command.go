// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package runner

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"strings"
	"text/template"
)

// Command allows building commands to execute using fluent-style api
type Command struct {
	command   string
	params    map[string]interface{}
	variables []string
	stream    bool
	stderr    bool
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
		if err := template.Must(template.New("").Parse(c.command)).Execute(&b, c.params); err != nil {
			return "", err
		}
		c.command = b.String()
	}

	cmd := exec.Command("/usr/bin/env", "bash", "-c", c.command)
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
