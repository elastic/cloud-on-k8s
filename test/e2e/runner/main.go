// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
)

func main() {
	tests, err := filepath.Glob("*.test")
	if err != nil {
		log.Fatal(err)
	}

	wd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	for _, t := range tests {
		args := []string{"-t", filepath.Join(wd, t), "-test.v", "-test.failfast", "-test.timeout=4h", "-test.parallel=1"}
		args = append(args, os.Args[1:]...)
		cmd := exec.Command(filepath.Join(wd, "test2json"), args...) //nolint:gosec
		cmd.Stderr = os.Stderr
		cmd.Stdout = os.Stdout
		if err := cmd.Run(); err != nil {
			log.Println(fmt.Errorf("while running %s: %w", t, err))
		}
	}
}
