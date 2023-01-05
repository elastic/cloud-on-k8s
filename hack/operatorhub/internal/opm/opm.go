// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package opm

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/operator-framework/operator-registry/pkg/lib/bundle"
	"github.com/sirupsen/logrus"
)

// GenerateConfig is the configuration for generating
// a bundle using the operator registry tooling.
type GenerateConfig struct {
	LocalDirectory  string
	OutputDirectory string
}

// GenerateBundle is used to build the operator bundle image to publish on OperatorHub.
func GenerateBundle(conf GenerateConfig) error {
	if _, err := os.Stat(conf.LocalDirectory); err != nil && os.IsNotExist(err) {
		os.MkdirAll(conf.LocalDirectory, 0700)
	}
	log.Printf("Generating operator bundle image for publishing ")
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
		log.Println("ⅹ")
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
				return fmt.Errorf("while deleting file %s: %w", file, err)
			}
		}
	}

	log.Println("✓")
	return nil
}

// EnsureAnnotations will ensure that the required annotations exist within the given file.
func EnsureAnnotations(file string, supportedVersions string) error {
	requiredAnnotations := []string{
		fmt.Sprintf(`  com.redhat.openshift.versions: %s`, supportedVersions),
	}
	b, err := ioutil.ReadFile(file)
	if err != nil {
		return fmt.Errorf("while reading file (%s): %w", file, err)
	}
	contents := string(b)
	for _, annotation := range requiredAnnotations {
		if !strings.Contains(contents, annotation) {
			if err = appendToFile(file, annotation); err != nil {
				return err
			}
		}
	}
	return nil
}

// EnsureLabels will ensure that the required labels exist within the given file.
func EnsureLabels(file string, supportedVersions string) error {
	requiredLabels := []string{
		fmt.Sprintf(`LABEL com.redhat.openshift.versions=%s`, supportedVersions),
		`LABEL com.redhat.delivery.backport=false`,
		`LABEL com.redhat.delivery.operator.bundle=true`,
	}
	b, err := ioutil.ReadFile(file)
	if err != nil {
		return fmt.Errorf("while reading file (%s): %w", file, err)
	}
	contents := string(b)
	for _, label := range requiredLabels {
		if !strings.Contains(contents, label) {
			if err = appendToFile(file, label); err != nil {
				return err
			}
		}
	}
	return nil
}

func appendToFile(file, data string) error {
	f, err := os.OpenFile(file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("while opening file (%s) for writing: %w", file, err)
	}
	defer f.Close()
	if _, err := f.WriteString(data + "\n"); err != nil {
		return fmt.Errorf("while appending to file (%s): %w", file, err)
	}
	return nil
}
