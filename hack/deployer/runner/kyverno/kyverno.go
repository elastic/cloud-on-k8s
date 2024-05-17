// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package kyverno

import (
	_ "embed"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/elastic/cloud-on-k8s/v2/hack/deployer/exec"
)

//go:embed install/kyverno.yaml
var installerManifest string

//go:embed install/policies.yaml
var policiesManifest string

//go:embed install/gke-policies.yaml
var GKEPolicies string

const (
	waitForKyvernoDeployment = `wait deployment kyverno -n kyverno --for condition=Available=True --timeout=15m`
)

func Install(globalKubectlOptions ...string) error {
	k := NewKubectl(globalKubectlOptions...)
	// Kyverno related manifests are stored in a temporary directory
	dir, err := os.MkdirTemp("", "kyverno")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	log.Println("Installing Kyverno")
	if err := apply(k, dir, installerManifest, "install.yaml"); err != nil {
		return err
	}
	log.Println("Waiting for Kyverno Pod to be ready...")
	if err := k.NewCommand(waitForKyvernoDeployment).Run(); err != nil {
		return err
	}

	log.Println("Installing Kyverno policies")
	if err := apply(k, dir, policiesManifest, "policies.yaml"); err != nil {
		return err
	}

	log.Println("Kyverno successfully installed")
	return nil
}

func apply(k *Kubectl, workDir string, content string, tmpFilename string) error {
	path := filepath.Join(workDir, tmpFilename)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		return err
	}
	return k.NewCommand(fmt.Sprintf(`apply -f %s`, path)).Run()
}

type Kubectl struct {
	prefix string
}

func NewKubectl(globalKubectlOptions ...string) *Kubectl {
	if len(globalKubectlOptions) == 0 {
		return &Kubectl{prefix: "kubectl"}
	}
	return &Kubectl{prefix: fmt.Sprintf("%s %s", "kubectl", strings.Join(globalKubectlOptions, " "))}
}

func (k *Kubectl) NewCommand(command string) *exec.Command {
	cmd := fmt.Sprintf("%s %s", k.prefix, command)
	return exec.NewCommand(cmd)
}
