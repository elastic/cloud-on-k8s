// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package runner

import (
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

var _ DriverFactory = (*K3dDriverFactory)(nil)

func (k K3dDriverFactory) Create(plan Plan) (Driver, error) {
	dockerSocket, err := getDockerSocket()
	if err != nil {
		return nil, err
	}
	return &K3dDriver{
		plan:         plan,
		vaultClient:  vault.NewClientProvider(),
		clientImage:  plan.K3d.ClientImage,
		nodeImage:    plan.K3d.NodeImage,
		dockerSocket: dockerSocket,
	}, nil
}

type K3dDriver struct {
	plan         Plan
	clientImage  string
	vaultClient  vault.ClientProvider
	nodeImage    string
	dockerSocket string
}

func (k *K3dDriver) Execute() error {
	switch k.plan.Operation {
	case CreateAction:
		if err := k.create(); err != nil {
			return err
		}
		if k.plan.Bucket != nil {
			if err := k.createBucket(); err != nil {
				return err
			}
		}
		return nil
	case DeleteAction:
		if k.plan.Bucket != nil {
			if err := k.deleteBucket(); err != nil {
				log.Printf("warning: bucket deletion failed: %v", err)
			}
		}
		return k.delete()
	}
	return nil
}

func (k *K3dDriver) create() error {
	// Do not have k3d modify the kubeconfig by default, as it ends up owned by the root user.
	cmd := k.cmd("cluster", "create", "--image", k.plan.K3d.NodeImage, "--kubeconfig-update-default=false", "--kubeconfig-switch-context=false")
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

	clusterName := fmt.Sprintf("%s-%s", "k3d", k.plan.ClusterName)
	userName := fmt.Sprintf("admin@%s", clusterName)
	// First try to remove any existing kubeconfig entries to ensure a clean merge
	if err := removeKubeconfig(clusterName, clusterName, userName); err != nil {
		return err
	}

	// Call mergeKubeconfig to properly merge the kubeconfig without incorrectly overwriting ownership.
	if err := mergeKubeconfig(kubeCfg.Name()); err != nil {
		return err
	}

	// Activate the kubeconfig context to replicate the behavior of the other runners.
	if err := activateKubeconfig(clusterName); err != nil {
		return err
	}

	tmpStorageClass, err := k.createTmpStorageClass()
	if err != nil {
		return err
	}

	defer os.Remove(tmpStorageClass)

	return kubectl("--kubeconfig", kubeCfg.Name(), "apply", "-f", tmpStorageClass)
}

func (k *K3dDriver) delete() error {
	// The cluster delete operation will not properly remove the kubeconfig entry
	// as the /home directory isn't mounted in the k3d flow.
	cmd := k.cmd("cluster", "delete")
	if cmd == nil {
		return fmt.Errorf("failed to create k3d cluster")
	}
	if err := cmd.Run(); err != nil {
		return err
	}
	// Manually remove the kubeconfig entry.
	clusterName := fmt.Sprintf("%s-%s", "k3d", k.plan.ClusterName)
	userName := fmt.Sprintf("admin@%s", clusterName)
	// ignore errors when removing clusters/users/contexts using k3d
	_ = removeKubeconfig(clusterName, clusterName, userName)
	return nil
}

func (k *K3dDriver) cmd(args ...string) *exec.Command {
	params := map[string]any{
		"ClusterName":    k.plan.ClusterName,
		"SharedVolume":   env.SharedVolumeName(),
		"K3dClientImage": k.clientImage,
		"K3dNodeImage":   k.nodeImage,
		"Args":           args,
	}

	// We need the docker socket so that k3d can bootstrap
	// --userns=host to support Docker daemon host configured to run containers only in user namespaces
	command := `docker run --rm \
		--userns=host \
		-v /var/run/docker.sock:` + k.dockerSocket + ` \
		-e HOME=/home \
		-e PATH=/ \
		{{.K3dClientImage}} \
		{{Join .Args " "}} {{.ClusterName}}`
	return exec.NewCommand(command).AsTemplate(params)
}

func (k *K3dDriver) getKubeConfig() (*os.File, error) {
	// Get kubeconfig from k3d binary
	output, err := k.cmd("kubeconfig", "get", k.plan.ClusterName).WithoutStreaming().Output()
	if err != nil {
		return nil, err
	}

	// Replace host.docker.internal with 127.0.0.1 only if not running inside the CI container
	// and on macOS.
	if os.Getenv("CI") != "true" && runtime.GOOS == "darwin" {
		output = strings.ReplaceAll(output, "host.docker.internal", "127.0.0.1")
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
	tmpFile := filepath.Join(os.TempDir(), storageClassFileName)
	err := os.WriteFile(tmpFile, []byte(storageClass), fs.ModePerm)
	return tmpFile, err
}

func (k *K3dDriver) createBucket() error {
	mgr, err := newLocalGCSBucketManager(k.plan)
	if err != nil {
		return err
	}
	return mgr.Create()
}

func (k *K3dDriver) deleteBucket() error {
	mgr, err := newLocalGCSBucketManager(k.plan)
	if err != nil {
		return err
	}
	return mgr.Delete()
}

func (k *K3dDriver) Cleanup(string, time.Duration) error {
	return fmt.Errorf("unimplemented")
}

var _ Driver = (*K3dDriver)(nil)
