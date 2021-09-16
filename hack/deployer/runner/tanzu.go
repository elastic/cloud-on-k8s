// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package runner

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"text/template"
)

const (
	TanzuDriverID                 = "tanzu"
	TanzuInstallConfig            = "install-config.yaml"
	DefaultTanzuRunConfigTemplate = `id: tanzu-dev
overrides:
  clusterName: %s-dev-cluster
`
	TanzuInstallerConfigTemplate = `AZURE_CLIENT_ID: {{.AzureClientID}}  
AZURE_CLIENT_SECRET: {{.AzureClientSecret}}
AZURE_CONTROL_PLANE_MACHINE_TYPE: {{.AzureMachineType}} 
AZURE_LOCATION: {{.AzureLocation}}
AZURE_NODE_MACHINE_TYPE: {{.AzureMachineType}}
AZURE_NODE_SUBNET_CIDR: 10.0.1.0/24
AZURE_RESOURCE_GROUP: {{.ResourceGroup}}
AZURE_SSH_PUBLIC_KEY_B64: {{.SSHPubKey}}
AZURE_SUBSCRIPTION_ID: {{.AzureSubscriptionID}}
AZURE_TENANT_ID: {{.AzureTenantID}}
CLUSTER_PLAN: dev
ENABLE_CEIP_PARTICIPATION: "false"
IDENTITY_MANAGEMENT_TYPE: none
INFRASTRUCTURE_PROVIDER: azure
OS_ARCH: amd64
OS_NAME: ubuntu
OS_VERSION: "20.04"
TKG_HTTP_PROXY_ENABLED: "false"
CONTROL_PLANE_MACHINE_COUNT: 1
WORKER_MACHINE_COUNT: {{.NodeCount}}
`
)

func init() {
	drivers[TanzuDriverID] = &TanzuDriverFactory{}
}

type TanzuDriverFactory struct{}

func (t TanzuDriverFactory) Create(plan Plan) (Driver, error) {
	if plan.VaultInfo == nil {
		return nil, fmt.Errorf("no vault credentials provided")
	}
	vaultClient, err := NewClient(*plan.VaultInfo)
	if err != nil {
		return nil, err
	}

	credentials, err := newAzureCredentials(vaultClient)
	if err != nil {
		return nil, err
	}

	// we have some legacy config in vault that probably should not be there: container registry name
	// is not strictly sensitive information
	acrName, err := vaultClient.Get(AksVaultPath, "acr-name")
	if err != nil {
		return nil, err
	}

	// users can optionally provide their SSH pubkey to log into hosts provisioned by this driver
	// however we need a pubkey in any case to be able to run the installer, thus we have a default in vault.
	if plan.Tanzu.SSHPubKey == "" {
		key, err := vaultClient.Get("secret/devops-ci/cloud-on-k8s/ci-tanzu-k8s-operator", "ssh_public_key")
		if err != nil {
			return nil, err
		}
		plan.Tanzu.SSHPubKey = key
	}

	// we use the Azure resource group to simplify garbage collecting the created resources on delete, if users set this
	// value they have to be aware that the referenced resource group will be deleted.
	if plan.Tanzu.ResourceGroup == "" {
		plan.Tanzu.ResourceGroup = plan.ClusterName
	}

	return &TanzuDriver{
		plan:             plan,
		azureCredentials: credentials,
		acrName:          acrName,
	}, nil
}

var _ DriverFactory = &TanzuDriverFactory{}

type TanzuDriver struct {
	plan             Plan
	acrName          string
	azureCredentials azureCredentials
	// runtime state
	installerStateDir string
}

func (t *TanzuDriver) Execute() error {
	if err := run(t.setup()); err != nil {
		return err
	}

	// we assume if the resource group exists that the management/workload clusters
	// have been deployed. This is not a guarantee however that they are functional.
	newDeployment, err := t.ensureResourceGroup()
	if err != nil {
		return err
	}

	switch t.plan.Operation {
	case CreateAction:
		if !newDeployment {
			log.Println("Not creating cluster because it exists")
			return nil
		}
		return t.create()
	case DeleteAction:
		// always attempt deletion
		return t.delete()
	}
	return nil
}

func (t *TanzuDriver) GetCredentials() error {
	if err := run(t.setup()); err != nil {
		return err
	}
	return t.copyKubeconfig()
}

// copyKubeconfig extracts the kubeconfig for the workload cluster out of the installer state and
// copies it to the main kube config (typically $HOME/.kube/config) where it is merged with existing configs.
func (t *TanzuDriver) copyKubeconfig() error {
	workloadConfigFile := "workload-kubeconfig"
	tanzuContainerKubeconfigPath := filepath.Join("/root", workloadConfigFile)
	if err := t.dockerizedTanzuCmd("cluster", "kubeconfig", "get", t.plan.ClusterName,
		"--admin", "--export-file", tanzuContainerKubeconfigPath).Run(); err != nil {
		return err
	}
	ciContainerKubeconfigPath := filepath.Join(t.installerStateDir, workloadConfigFile)
	return mergeKubeconfig(ciContainerKubeconfigPath)
}

// perpareCLI prepares the tanzu CLI by installing the necessary plugins.
func (t *TanzuDriver) perpareCLI() error {
	log.Println("Installing Tanzu CLI plugins")
	return t.dockerizedTanzuCmd("plugin", "install", "--local", "/", "all").Run()
}

// teardownCLI uninstall the plugins again mainly to make the footprint of the installer state smaller and to avoid
// permissions issues when restoring that state as it also contains executables.
func (t *TanzuDriver) teardownCLI() error {
	return t.dockerizedTanzuCmd("plugin", "clean").Run()
}

// ensureWorkDir setups the workdir in a place that is accessible from the calling context (on CI deployer is called from
// within another container). Users can also just set a fixed workdir via config which is useful for debugging.
func (t *TanzuDriver) ensureWorkDir() error {
	if t.installerStateDir != "" {
		// already initialised
		return nil
	}
	workDir := t.plan.Tanzu.WorkDir
	if workDir == "" {
		// base work dir in HOME dir otherwise mounting to container won't work without further settings adjustment
		// in macOS in local mode. In CI mode we need the workdir to be in the volume shared between containers.
		// having the work dir in HOME also underlines the importance of the work dir contents. The work dir is the only
		// source to cleanly uninstall the cluster should the rsync fail.
		var err error
		workDir, err = ioutil.TempDir(os.Getenv("HOME"), t.plan.ClusterName)
		if err != nil {
			return err
		}
		log.Printf("Defaulting WorkDir: %s", workDir)
	}

	if err := os.MkdirAll(workDir, os.ModePerm); err != nil {
		return err
	}
	t.installerStateDir = workDir
	log.Printf("Using installer state dir: %s", workDir)
	return nil
}

// create creates the Tanzu management and workload clusters and installs the storage class needed for e2e testing.
func (t *TanzuDriver) create() error {
	log.Println("Creating management cluster")
	err := t.createInstallerConfig()
	if err != nil {
		return err
	}
	// run clean up and state upload to be able to continue to run operations later with a fresh Docker container
	defer t.suspend()

	cfgPathInContainer := filepath.Join("/root", TanzuInstallConfig)
	if err := t.dockerizedTanzuCmd("management-cluster", "create", "--file", cfgPathInContainer).Run(); err != nil {
		return err
	}
	log.Println("Creating workload cluster")
	if err := t.dockerizedTanzuCmd("cluster", "create", t.plan.ClusterName, "--file", cfgPathInContainer).Run(); err != nil {
		return err
	}
	return run([]func() error{
		t.copyKubeconfig,
		func() error {
			return setupDisks(t.plan)
		},
		createStorageClass,
	})
}

// createInstallerConfig renders the installer config into a file. This config can be used for both workload and management cluster
// Tanzu ignores the worker machine count setting for management clusters.
func (t *TanzuDriver) createInstallerConfig() error {
	params := map[string]interface{}{
		"AzureClientID":       t.azureCredentials.ClientID,
		"AzureClientSecret":   t.azureCredentials.ClientSecret,
		"AzureMachineType":    t.plan.MachineType,
		"AzureLocation":       t.plan.Tanzu.Location,
		"ResourceGroup":       t.plan.Tanzu.ResourceGroup,
		"SSHPubKey":           t.plan.Tanzu.SSHPubKey,
		"AzureSubscriptionID": t.azureCredentials.SubscriptionID,
		"AzureTenantID":       t.azureCredentials.TenantID,
		"NodeCount":           t.plan.Tanzu.NodeCount,
	}
	var cfg bytes.Buffer
	if err := template.Must(template.New("tanzu-mgmt").Parse(TanzuInstallerConfigTemplate)).Execute(&cfg, params); err != nil {
		return err
	}

	return ioutil.WriteFile(t.installerConfigFilePath(), cfg.Bytes(), 0600)
}

// installerConfigFilePath returns the path to the installer config valid in the context of deployer.
func (t *TanzuDriver) installerConfigFilePath() string {
	return filepath.Join(t.installerStateDir, TanzuInstallConfig)
}

// delete deletes the created resources simply by removing the Azure resource group
func (t *TanzuDriver) delete() error {
	return run([]func() error{
		// shortcut deletion by not attempting deletion on application level but just delete the infrastructure
		t.deleteResourceGroup,
		t.deleteStorageContainer,
		func() error {
			user := fmt.Sprintf("%s-admin", t.plan.ClusterName)
			context := fmt.Sprintf("%s@%s", user, t.plan.ClusterName)
			return removeKubeconfig(context, t.plan.ClusterName, user)
		},
	})
}

// dockerizedTanzuCmd runs a tanzu CLI command inside a Docker container based off the Tanzu installer image.
func (t *TanzuDriver) dockerizedTanzuCmd(args ...string) *Command {
	params := map[string]interface{}{
		"SharedVolume":        filepath.Join(SharedVolumeName(), filepath.Base(t.installerStateDir)),
		"TanzuCLIDockerImage": t.plan.Tanzu.InstallerImage,
		"Args":                args,
	}
	// We are mounting the shared volume into the installer container and configure it to be the HOME directory
	// this is mainly so that we can save the installer state afterwards correctly as the Tanzu CLI is does not allow
	// customizing where it saves its state. It always uses $HOME.
	// We are mounting tmp as the installer needs a scratch space and writing into the container won't work
	// We need host networking because the Tanzu installer will try to connect to a kind cluster it boots up
	cmd := NewCommand(`docker run --rm \
		-v {{.SharedVolume}}:/root \
		-v /var/run/docker.sock:/var/run/docker.sock \
		-v /tmp:/tmp \
		-e HOME=/root \
		-e PATH=/ \
		--network host \
		{{.TanzuCLIDockerImage}} \
		/tanzu {{Join .Args " "}}`)
	return cmd.AsTemplate(params)
}

// setup is a list of functions that need to run before any installer commands can run.
func (t *TanzuDriver) setup() []func() error {
	return []func() error{
		t.loginToAzure,
		t.loginToContainerRegistry,
		t.ensureWorkDir,
		t.ensureStorageContainer,
		t.restoreInstallerState,
		t.perpareCLI,
	}
}

// suspend persists the installer state in a storage container so that further operations that need it can be run later.
func (t *TanzuDriver) suspend() {
	// clean up sensitive bits
	if err := os.Remove(t.installerConfigFilePath()); err != nil {
		log.Println(err.Error())
	}

	// uninstall plugins to reduce state size by ~ 400MB and avoid issues with missing execute permissions after restore
	if err := t.teardownCLI(); err != nil {
		log.Println(err.Error())
	}

	// persist state to storage bucket to be able to continue later to either get credentials or
	// to delete resources
	if err := t.persistInstallerState(); err != nil {
		log.Println(err.Error())
	}
}

// loginToAzure we use Azure as the infrastructure provider for Tanzu testing.
func (t *TanzuDriver) loginToAzure() error {
	log.Println("Logging in to Azure")
	return azureLogin(t.azureCredentials)
}

// loginToContainerRegistry we use a private container registry to make the Tanzu CLI available in CI.
func (t *TanzuDriver) loginToContainerRegistry() error {
	log.Println("Logging in to container registry")
	return NewCommand("az acr login --name {{.ContainerRegistry}}").
		AsTemplate(map[string]interface{}{
			"ContainerRegistry": t.acrName,
		}).Run()
}

// ensureResourceGroup checks for the existence of an Azure resource group (which we name unless overridden after the
// cluster we want to deploy)
func (t *TanzuDriver) ensureResourceGroup() (bool, error) {
	exists, err := NewCommand("az group exists --name {{.ResourceGroup}}").
		AsTemplate(map[string]interface{}{
			"ResourceGroup": t.plan.Tanzu.ResourceGroup,
		}).WithoutStreaming().OutputContainsAny("true")
	if err != nil || exists {
		return false, err
	}
	log.Println("Creating Azure resource group")
	err = NewCommand("az group create -l {{.Location}} --name {{.ResourceGroup}}").
		AsTemplate(map[string]interface{}{
			"Location":      t.plan.Tanzu.Location,
			"ResourceGroup": t.plan.Tanzu.ResourceGroup,
		}).WithoutStreaming().Run()
	return true, err
}

func (t *TanzuDriver) deleteResourceGroup() error {
	log.Println("Deleting Azure resource group")
	return NewCommand("az group delete --name {{.ResourceGroup}} -y").AsTemplate(map[string]interface{}{
		"ResourceGroup": t.plan.Tanzu.ResourceGroup,
	}).Run()
}

func (t *TanzuDriver) storageContainerExists() (bool, error) {
	return azureExistsCmd(NewCommand("az storage container exists --account-name cloudonk8s --name {{.StorageContainer}} --auth login").
		AsTemplate(map[string]interface{}{
			"StorageContainer": t.plan.ClusterName,
		}))
}

func (t *TanzuDriver) ensureStorageContainer() error {
	exists, err := t.storageContainerExists()
	if err != nil {
		return err
	}
	if exists {
		log.Println("Reusing existing storage container to persist installer state")
		return nil
	}
	log.Println("Creating new storage container to persist installer state")
	return NewCommand("az storage container create --account-name cloudonk8s --name {{.StorageContainer}} --auth login").
		AsTemplate(map[string]interface{}{
			"StorageContainer": t.plan.ClusterName,
		}).WithoutStreaming().Run()
}

func (t TanzuDriver) deleteStorageContainer() error {
	log.Println("Deleting Azure storage container")
	return NewCommand("az storage container delete --account-name cloudonk8s --name {{.StorageContainer}} --auth login").
		AsTemplate(map[string]interface{}{
			"StorageContainer": t.plan.ClusterName,
		}).WithoutStreaming().Run()
}

func (t *TanzuDriver) persistInstallerState() error {
	log.Println("Persisting installer state to Azure storage container")
	return NewCommand(`az storage azcopy blob sync -c {{.StorageContainer}} --account-name cloudonk8s -s "{{.InstallerStateDir}}"`).
		AsTemplate(map[string]interface{}{
			"StorageContainer":  t.plan.ClusterName,
			"InstallerStateDir": t.installerStateDir,
		}).WithoutStreaming().Run()
}

func (t *TanzuDriver) restoreInstallerState() error {
	log.Println("Restoring installer state from storage container if any")
	return NewCommand(`az storage azcopy blob download -c {{.StorageContainer}} --account-name cloudonk8s -s '*' -d "{{.InstallerStateDir}}" --recursive`).
		AsTemplate(map[string]interface{}{
			"StorageContainer":  t.plan.ClusterName,
			"InstallerStateDir": t.installerStateDir,
		}).WithoutStreaming().Run()
}

var _ Driver = &TanzuDriver{}
