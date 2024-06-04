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
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/blang/semver/v4"

	"github.com/elastic/cloud-on-k8s/v2/hack/deployer/exec"
	"github.com/elastic/cloud-on-k8s/v2/hack/deployer/runner/env"
	"github.com/elastic/cloud-on-k8s/v2/hack/deployer/runner/kyverno"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/vault"
)

const (
	KindDriverID = "kind"

	DefaultKindRunConfigTemplate = `id: kind-dev
overrides:
  clusterName: %s-dev-cluster
`
	// manifest is a kind cluster template
	// the explicit podSubnet definition can be removed as soon as https://github.com/kubernetes-sigs/kind/commit/60074a9e67ddc8d35d3468ab137358b62a4cf723
	// will be available in a released version of kind and we don't rely on older versions anymore
	manifest = `---
kind: Cluster
apiVersion: {{.APIVersion}}
networking:
  ipFamily: {{.IPFamily}}
{{- if eq .IPFamily "ipv6" }}
  podSubnet: "fd00:10:244::/56"
{{- end}}
nodes:
  - role: control-plane
{{- range .WorkerNames }}
  - role: worker
{{- end}}
`
	storageClassFileName = "storageclass.yaml"
	storageClass         = `apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  annotations:
    storageclass.beta.kubernetes.io/is-default-class: "true"
  name: e2e-default
provisioner: rancher.io/local-path
volumeBindingMode: WaitForFirstConsumer
reclaimPolicy: Delete
`
)

func init() {
	drivers[KindDriverID] = &KindDriverFactory{}
}

type KindDriverFactory struct{}

var _ DriverFactory = &KindDriverFactory{}

func (k KindDriverFactory) Create(plan Plan) (Driver, error) {
	c, err := vault.NewClient()
	if err != nil {
		return nil, err
	}
	return &KindDriver{
		plan:        plan,
		vaultClient: c,
	}, nil
}

type KindDriver struct {
	plan        Plan
	clientImage string
	vaultClient vault.Client
}

func (k *KindDriver) Execute() error {
	if err := k.ensureClientImage(); err != nil {
		return err
	}

	switch k.plan.Operation {
	case CreateAction:
		return k.create()
	case DeleteAction:
		return k.delete()
	}
	return nil
}

func (k *KindDriver) create() error {
	// Write manifest to temporary file
	tmpManifest, err := k.createTmpManifest()
	if err != nil {
		return err
	}
	defer os.Remove(tmpManifest.Name())

	// Delete any previous e2e kind cluster with the same name
	err = k.delete()
	if err != nil {
		return err
	}

	err = k.cmd("create", "cluster", "--config", k.inContainerName(tmpManifest), "--retain", "--image", k.plan.Kind.NodeImage).Run()
	if err != nil {
		return err
	}

	// Get kubeconfig from kind
	kubeCfg, err := k.getKubeConfig()
	if err != nil {
		return err
	}
	defer os.Remove(kubeCfg.Name())

	// Delete standard storage class but ignore error if not found
	if err := kubectl("--kubeconfig", kubeCfg.Name(), "delete", "storageclass", "standard"); err != nil {
		return err
	}

	tmpStorageClass, err := k.createTmpStorageClass()
	if err != nil {
		return err
	}

	if k.plan.EnforceSecurityPolicies {
		if err := kyverno.Install("--kubeconfig", kubeCfg.Name()); err != nil {
			return err
		}
	}

	return kubectl("--kubeconfig", kubeCfg.Name(), "apply", "-f", tmpStorageClass)
}

func (k *KindDriver) inContainerName(file *os.File) string {
	return filepath.Join("/home", filepath.Base(file.Name()))
}

func kubectl(arg ...string) error {
	output, err := exec.NewCommand(`kubectl {{Join .Args " "}}`).AsTemplate(map[string]interface{}{"Args": arg}).Output()
	if err != nil && strings.Contains(output, "Error from server (NotFound)") {
		log.Printf("Ignoring NotFound error for command: %v\n", arg)
		return nil // ignore not found errors
	}
	return err
}

func (k *KindDriver) delete() error {
	return k.cmd("delete", "cluster").Run()
}

func (k *KindDriver) createTmpManifest() (*os.File, error) {
	// HOME is shared between CI container and Kind container
	f, err := os.CreateTemp(os.Getenv("HOME"), "kind-cluster")
	if err != nil {
		return nil, err
	}

	tmpl, err := template.New("cluster.yaml").Parse(manifest)
	if err != nil {
		return nil, err
	}

	type tplArgs struct {
		APIVersion  string
		IPFamily    string
		WorkerNames []string
	}
	ta := tplArgs{
		APIVersion:  k.apiVersion(),
		IPFamily:    k.plan.Kind.IPFamily,
		WorkerNames: k.workerNames(),
	}
	return f, tmpl.Execute(f, ta)
}

func (k *KindDriver) workerNames() []string {
	var names []string
	// kind has the following naming scheme <cluster-name>-worker, <cluster-name>-worker2 etc
	// this is not configurable and thus explains the awkward name construction here
	for i := 0; i < k.plan.Kind.NodeCount; i++ {
		suffix := ""
		if i > 0 {
			suffix = strconv.Itoa(i + 1)
		}
		names = append(names, fmt.Sprintf("%s-worker%s", k.plan.ClusterName, suffix))
	}
	return names
}

func (k *KindDriver) cmd(args ...string) *exec.Command {
	params := map[string]interface{}{
		"SharedVolume":    env.SharedVolumeName(),
		"KindClientImage": k.clientImage,
		"ClusterName":     k.plan.ClusterName,
		"Args":            args,
	}

	// on macOS, the docker socket is located in $HOME
	dockerSocket := "/var/run/docker.sock"
	if runtime.GOOS == "darwin" {
		dockerSocket = "$HOME/.docker/run/docker.sock"
	}
	// We need the docker socket so that kind can bootstrap
	// --userns=host to support Docker daemon host configured to run containers only in user namespaces
	cmd := exec.NewCommand(`docker run --rm \
		--userns=host \
		-v {{.SharedVolume}}:/home \
		-v /var/run/docker.sock:` + dockerSocket + ` \
		-e HOME=/home \
		-e PATH=/ \
		{{.KindClientImage}} \
		/kind {{Join .Args " "}} --name {{.ClusterName}}`)
	return cmd.AsTemplate(params)
}

func (k *KindDriver) apiVersion() string {
	apiVersion := "kind.sigs.k8s.io/v1alpha3"
	v := semver.MustParse(k.plan.ClientVersion)
	if v.GTE(semver.MustParse("0.9.0")) {
		apiVersion = "kind.x-k8s.io/v1alpha4"
	}
	return apiVersion
}

func (k *KindDriver) getKubeConfig() (*os.File, error) {
	// Get kubeconfig from kind
	output, err := k.cmd("get", "kubeconfig").WithoutStreaming().Output()
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

func (k *KindDriver) GetCredentials() error {
	if err := k.ensureClientImage(); err != nil {
		return err
	}

	config, err := k.getKubeConfig()
	if err != nil {
		return err
	}
	defer os.Remove(config.Name())
	return mergeKubeconfig(config.Name())
}

func (k *KindDriver) createTmpStorageClass() (string, error) {
	tmpFile := filepath.Join(os.Getenv("HOME"), storageClassFileName)
	err := os.WriteFile(tmpFile, []byte(storageClass), fs.ModePerm)
	return tmpFile, err
}

func (k *KindDriver) ensureClientImage() error {
	image, err := ensureClientImage(KindDriverID, k.vaultClient, k.plan.ClientVersion, k.plan.ClientBuildDefDir)
	if err != nil {
		return err
	}
	k.clientImage = image
	return nil
}

func (k *KindDriver) Cleanup(string, time.Duration) error {
	return fmt.Errorf("unimplemented")
}

var _ Driver = &KindDriver{}
