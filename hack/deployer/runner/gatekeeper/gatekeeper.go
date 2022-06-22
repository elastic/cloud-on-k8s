// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package gatekeeper

import (
	_ "embed"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/elastic/cloud-on-k8s/hack/deployer/exec"
)

//go:embed install/gatekeeper.yaml
var install string

//go:embed install/library.yaml
var library string

//go:embed install/default-constraints.yaml
var defaultConstraints string

const (
	waitForAuditPod           = `kubectl wait deployment gatekeeper-audit -n gatekeeper-system --for condition=Available=True --timeout=60s`
	waitForControllers        = `kubectl wait deployment gatekeeper-controller-manager -n gatekeeper-system --for condition=Available=True --timeout=60s`
	createdConstraintTemplate = `kubectl get ConstraintTemplate -o go-template='{{range .items}}{{or .status.created "false"}}{{","}}{{end}}'`

	maxRetry          = 30
	retryWaitDuration = 2 * time.Second
)

func Install(installDefaultConstraints bool) error {
	// Gatekeeper related manifests are stored in a temporary directory
	dir, err := ioutil.TempDir("", "gatekeeper")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	// Install Gatekeeper
	log.Println("Installing Gatekeeper...")
	if err := apply(dir, install, "install.yaml"); err != nil {
		return err
	}
	log.Println("Waiting for Gatekeeper to be ready...")
	if err := exec.NewCommand(waitForAuditPod).Run(); err != nil {
		return err
	}
	if err := exec.NewCommand(waitForControllers).Run(); err != nil {
		return err
	}

	// Install constraints templates library
	log.Println("Installing Gatekeeper library")
	if err := apply(dir, library, "library.yaml"); err != nil {
		return err
	}

	log.Println("Waiting for Gatekeeper library to be available")
	retry := 0
	for {
		constraintTemplateNotCreated, err := exec.NewCommand(createdConstraintTemplate).WithoutStreaming().OutputContainsAny("false")
		if err != nil {
			return nil
		}
		if !constraintTemplateNotCreated {
			break
		}
		retry++
		if retry > maxRetry {
			return errors.New("timeout while waiting for ConstraintTemplate to be installed")
		}
		time.Sleep(retryWaitDuration)
	}

	if installDefaultConstraints {
		log.Println("Installing default Gatekeeper constraints")
		if err := apply(dir, defaultConstraints, "default-constraints.yaml"); err != nil {
			return err
		}
	}
	log.Println("Gatekeeper successfully installed")
	return nil
}

func apply(workDir string, content string, tmpFilename string) error {
	path := filepath.Join(workDir, tmpFilename)
	if err := ioutil.WriteFile(path, []byte(content), 0644); err != nil {
		return err
	}
	return exec.NewCommand(fmt.Sprintf(`kubectl apply -f %s`, path)).Run()
}
