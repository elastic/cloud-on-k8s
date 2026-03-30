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

	"github.com/elastic/cloud-on-k8s/v3/hack/deployer/exec"
)

//go:embed install/kyverno.yaml
var installerManifest string

//go:embed install/policies.yaml
var policiesManifest string

//go:embed install/gke-policies.yaml
var GKEPolicies string

const (
	waitForKyvernoAdmissionDeployment  = `wait deployment kyverno-admission-controller -n kyverno --for condition=Available=True --timeout=20m`
	waitForKyvernoBackgroundDeployment = `wait deployment kyverno-background-controller -n kyverno --for condition=Available=True --timeout=20m`
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
	if err := apply(k, dir, installerManifest, "install.yaml", "--server-side"); err != nil {
		return err
	}
	log.Println("Waiting for Kyverno Pod to be ready...")
	if err := k.NewCommand(waitForKyvernoAdmissionDeployment).Run(); err != nil {
		return err
	}
	if err := k.NewCommand(waitForKyvernoBackgroundDeployment).Run(); err != nil {
		return err
	}

	log.Println("Installing Kyverno policies")
	if err := apply(k, dir, policiesManifest, "policies.yaml"); err != nil {
		return err
	}

	log.Println("Kyverno successfully installed")
	return nil
}

func apply(k *Kubectl, workDir string, content string, tmpFilename string, args ...string) error {
	path := filepath.Join(workDir, tmpFilename)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return err
	}
	cmd := fmt.Sprintf(`apply -f %s`, path)
	if len(args) > 0 {
		cmd = fmt.Sprintf(`apply %s -f %s`, strings.Join(args, " "), path)
	}
	return k.NewCommand(cmd).Run()
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
