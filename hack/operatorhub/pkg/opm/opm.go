// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package opm

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/operator-framework/operator-registry/pkg/lib/bundle"
	"github.com/pterm/pterm"
	"github.com/sirupsen/logrus"
)

// GenerateConfig is the configuration for generating a bundle using the operator
// registry tooling.
type GenerateConfig struct {
	LocalDirectory  string
	OutputDirectory string
}

// GenerateBundle is used to build the operator bundle image to publish on the OpenShift OperatorHub.
func GenerateBundle(conf GenerateConfig) error {
	if _, err := os.Stat(conf.LocalDirectory); err != nil && os.IsNotExist(err) {
		os.MkdirAll(conf.LocalDirectory, 0700)
	}
	pterm.Printf("Generating operator bundle image for publishing ")
	// Disable logging of unnecessary information during bundle creation
	logrus.SetLevel(logrus.WarnLevel)
	err := bundle.GenerateFunc(
		conf.LocalDirectory,
		conf.OutputDirectory,
		"",
		"",
		"",
		true,
	)
	if err != nil {
		pterm.Println(pterm.Red("ⅹ"))
		return fmt.Errorf("during opm generate: %w", err)
	}

	files, err := filepath.Glob(filepath.Join(conf.LocalDirectory, "*"))
	if err != nil {
		return fmt.Errorf("listing all files in %s: %w", conf.LocalDirectory, err)
	}

	for _, file := range files {
		if strings.HasSuffix(file, ".yaml") || strings.HasSuffix(file, ".yml") {
			err = os.RemoveAll(file)
			if err != nil {
				return fmt.Errorf("deleting file %s: %w", file, err)
			}
		}
	}

	pterm.Println(pterm.Green("✓"))
	return nil
}

// EnsureAnnotations will ensure that the required annotations exist within the given file.
func EnsureAnnotations(file string, supportedVersions string) error {
	requiredAnnotations := []string{
		fmt.Sprintf(`  com.redhat.openshift.versions: %s`, supportedVersions),
	}
	b, err := ioutil.ReadFile(file)
	if err != nil {
		return fmt.Errorf("failed to read file (%s): %w", file, err)
	}
	contents := string(b)
	for _, label := range requiredAnnotations {
		if !strings.Contains(contents, label) {
			if err = appendAnnotation(file, label); err != nil {
				return err
			}
		}
	}
	return nil
}

func appendAnnotation(file, annotation string) error {
	f, err := os.OpenFile(file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open file (%s) for writing: %w", file, err)
	}
	defer f.Close()
	if _, err := f.WriteString(annotation + "\n"); err != nil {
		return fmt.Errorf("failed to append to file (%s): %w", file, err)
	}
	return nil
}
