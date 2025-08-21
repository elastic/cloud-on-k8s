// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package runner

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/elastic/cloud-on-k8s/v3/hack/deployer/exec"
	"github.com/elastic/cloud-on-k8s/v3/hack/deployer/runner/env"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/vault"
)

const (
	K3dDriverID = "k3d"

	DefaultK3dRunConfigTemplate = `id: k3d-dev
overrides:
  clusterName: %s-dev-cluster
`
)

func init() {
	drivers[K3dDriverID] = &K3dDriverFactory{}
}

type K3dDriverFactory struct{}

var _ DriverFactory = &K3dDriverFactory{}

func (k K3dDriverFactory) Create(plan Plan) (Driver, error) {
	return &K3dDriver{
		plan:        plan,
		vaultClient: vault.NewClientProvider(),
		clientImage: plan.K3d.ClientImage,
		nodeImage:   plan.K3d.NodeImage,
	}, nil
}

type K3dDriver struct {
	plan        Plan
	clientImage string
	vaultClient vault.ClientProvider
	nodeImage   string
}

func (k *K3dDriver) Execute() error {
	switch k.plan.Operation {
	case CreateAction:
		return k.create()
	case DeleteAction:
		return k.delete()
	}
	return nil
}

func (k *K3dDriver) create() error {
	cmd := k.cmd("cluster", "create", "--image", k.plan.K3d.NodeImage)
	if cmd == nil {
		return fmt.Errorf("failed to create k3d cluster")
	}
	err := cmd.Run()
	if err != nil {
		return err
	}

	// Get kubeconfig from k3d
	kubeCfg, err := k.getKubeConfig()
	if err != nil {
		return err
	}
	defer os.Remove(kubeCfg.Name())

	if err := kubectl("--kubeconfig", kubeCfg.Name(), "delete", "storageclass", "standard"); err != nil {
		return err
	}

	tmpStorageClass, err := k.createTmpStorageClass()
	if err != nil {
		return err
	}

	return kubectl("--kubeconfig", kubeCfg.Name(), "apply", "-f", tmpStorageClass)
}

func (k *K3dDriver) delete() error {
	return fmt.Errorf("unimplemented")
}

func (k *K3dDriver) cmd(args ...string) *exec.Command {
	params := map[string]interface{}{
		"ClusterName":    k.plan.ClusterName,
		"SharedVolume":   env.SharedVolumeName(),
		"K3dClientImage": k.clientImage,
		"K3dNodeImage":   k.nodeImage,
		"Args":           args,
	}

	// on macOS, the docker socket is located in $HOME
	dockerSocket := "/var/run/docker.sock"
	if runtime.GOOS == "darwin" {
		dockerSocket = "$HOME/.docker/run/docker.sock"
	}
	// We need the docker socket so that kind can bootstrap
	// --userns=host to support Docker daemon host configured to run containers only in user namespaces
	command := `docker run --rm \
		--userns=host \
		-v {{.SharedVolume}}:/home \
		-v /var/run/docker.sock:` + dockerSocket + ` \
		-e HOME=/home \
		-e PATH=/ \
		{{.K3dClientImage}} \
		{{Join .Args " "}} {{.ClusterName}}`
	cmd := exec.NewCommand(command)
	cmd = cmd.AsTemplate(params)
	return cmd
}

func (k *K3dDriver) getKubeConfig() (*os.File, error) {
	// Get kubeconfig from k3d binary
	output, err := k.cmd("kubeconfig", "get").WithoutStreaming().Output()
	if err != nil {
		return nil, err
	}

	// Persist kubeconfig for reliability in following kubectl commands
	kubeCfg, err := os.CreateTemp("", "kubeconfig")
	if err != nil {
		return nil, err
	}

	_, err = kubeCfg.Write([]byte(output))
	if err != nil {
		return nil, err
	}
	return kubeCfg, nil
}

func (k *K3dDriver) GetCredentials() error {
	config, err := k.getKubeConfig()
	if err != nil {
		return err
	}
	defer os.Remove(config.Name())
	return mergeKubeconfig(config.Name())
}

func (k *K3dDriver) createTmpStorageClass() (string, error) {
	tmpFile := filepath.Join(os.Getenv("HOME"), storageClassFileName)
	err := os.WriteFile(tmpFile, []byte(storageClass), fs.ModePerm)
	return tmpFile, err
}

func (k *K3dDriver) Cleanup(string, time.Duration) error {
	return fmt.Errorf("unimplemented")
}

var _ Driver = &K3dDriver{}
