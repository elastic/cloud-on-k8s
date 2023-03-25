// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package runner

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/elastic/cloud-on-k8s/v2/hack/deployer/exec"
	"github.com/elastic/cloud-on-k8s/v2/hack/deployer/runner/azure"
	"github.com/elastic/cloud-on-k8s/v2/hack/deployer/runner/env"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/vault"
)

const (
	TanzuDriverID                 = "tanzu"
	TanzuInstallConfig            = "install-config.yaml"
	TanzuVaultPath                = "ci-tanzu-k8s-operator"
	DefaultTanzuRunConfigTemplate = `id: tanzu-dev
overrides:
  clusterName: %s-dev-cluster
`
	TanzuInstallerConfigTemplate = `AZURE_CLIENT_ID: {{.AzureClientID}}  
AZURE_CLIENT_SECRET: {{.AzureClientSecret}}
AZURE_CONTROL_PLANE_MACHINE_TYPE: {{.AzureMachineType}} 
AZURE_LOCATION: {{.AzureLocation}}
AZURE_CUSTOM_TAGS: {{.AzureCustomTags}}
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
	c, err := vault.NewClient()
	if err != nil {
		return nil, err
	}

	credentials, err := azure.NewCredentials(c)
	if err != nil {
		return nil, err
	}

	// we have some legacy config in vault that probably should not be there: container registry name
	// is not strictly sensitive information
	acrName, err := vault.Get(c, azure.AKSVaultPath, "acr-name")
	if err != nil {
		return nil, err
	}

	// users can optionally provide their SSH pubkey to log into hosts provisioned by this driver
	// however we need a pubkey in any case to be able to run the installer, thus we have a default in vault.
	if plan.Tanzu.SSHPubKey == "" {
		key, err := vault.Get(c, TanzuVaultPath, "ssh_public_key")
		if err != nil {
			return nil, err
		}
		plan.Tanzu.SSHPubKey = key
	}

	storageAccount, err := vault.Get(c, TanzuVaultPath, "storage_account")
	if err != nil {
		return nil, err
	}

	// we use the Azure resource group to simplify garbage collecting the created resources on delete, if users set this
	// value they have to be aware that the referenced resource group will be deleted.
	if plan.Tanzu.ResourceGroup == "" {
		plan.Tanzu.ResourceGroup = plan.ClusterName
	}

	return &TanzuDriver{
		plan:                plan,
		azureCredentials:    credentials,
		azureStorageAccount: storageAccount,
		acrName:             acrName,
	}, nil
}

var _ DriverFactory = &TanzuDriverFactory{}

type TanzuDriver struct {
	plan                Plan
	acrName             string
	azureStorageAccount string
	azureCredentials    azure.Credentials
	// runtime state paths relative to deployers environment
	installerStateDirPath     string
	installerStateDirBasename string
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
	tanzuContainerKubeconfigPath := t.tanzuContainerPath(workloadConfigFile)
	if err := t.dockerizedTanzuCmd("cluster", "kubeconfig", "get", t.plan.ClusterName,
		"--admin", "--export-file", tanzuContainerKubeconfigPath).Run(); err != nil {
		return err
	}
	ciContainerKubeconfigPath := t.deployerContainerPath(workloadConfigFile)
	return mergeKubeconfig(ciContainerKubeconfigPath)
}

// prepareCLI prepares the tanzu/az CLI by installing the necessary plugins and setting up configuration
func (t *TanzuDriver) prepareCLI() error {
	log.Println("Installing Tanzu CLI plugins")
	// init also syncs the plugins
	return t.dockerizedTanzuCmd("init").Run()
}

// teardownCLI uninstall the plugins again mainly to make the footprint of the installer state smaller and to avoid
// permissions issues when restoring that state as it also contains executables.
func (t *TanzuDriver) teardownCLI() error {
	return t.dockerizedTanzuCmd("plugin", "clean").Run()
}

// ensureWorkDir setups the workdir in a place that is accessible from the calling context (on CI deployer is called from
// within another container). Users can also just set a fixed workdir via config which is useful for debugging.
func (t *TanzuDriver) ensureWorkDir() error {
	if t.installerStateDirPath != "" {
		// already initialised
		return nil
	}
	workDir := t.plan.Tanzu.WorkDir
	if workDir == "" {
		// base work dir in HOME dir otherwise mounting to container won't work without further settings adjustment
		// in macOS in local mode. In CI mode we need the workdir to be in the volume shared between containers.
		workDir = filepath.Join(os.Getenv("HOME"), t.plan.ClusterName)
		log.Printf("Defaulting WorkDir: %s", workDir)
	}

	if err := os.MkdirAll(workDir, os.ModePerm); err != nil {
		return err
	}
	t.installerStateDirPath = workDir
	t.installerStateDirBasename = filepath.Base(workDir)
	log.Printf("Using installer state dir: %s", workDir)
	return nil
}

// tanzuContainerPath returns an absolute path valid inside the Tanzu installer container using the given relative path.
func (t *TanzuDriver) tanzuContainerPath(path string) string {
	return filepath.Join("/root", t.installerStateDirBasename, path)
}

// deployerContainerPath returns an absolute path valid inside the deployer environment (container or not) using the give
// relative path.
func (t TanzuDriver) deployerContainerPath(path string) string {
	return filepath.Join(t.installerStateDirPath, path)
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

	cfgPathInContainer := t.tanzuContainerPath(TanzuInstallConfig)
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
		"AzureCustomTags":     strings.Join(toList(elasticTags), ","),
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

	return os.WriteFile(t.installerConfigFilePath(), cfg.Bytes(), 0600)
}

// installerConfigFilePath returns the path to the installer config valid in the context of deployer.
func (t *TanzuDriver) installerConfigFilePath() string {
	return t.deployerContainerPath(TanzuInstallConfig)
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
func (t *TanzuDriver) dockerizedTanzuCmd(args ...string) *exec.Command {
	params := map[string]interface{}{
		"SharedVolume":        env.SharedVolumeName(),
		"TanzuCLIDockerImage": t.plan.Tanzu.InstallerImage,
		"Home":                t.tanzuContainerPath(""),
		"Args":                args,
	}
	// We are mounting the shared volume into the installer container and configure it to be the HOME directory
	// this is mainly so that we can save the installer state afterwards correctly as the Tanzu CLI is does not allow
	// customizing where it saves its state. It always uses $HOME.
	// We are mounting tmp as the installer needs a scratch space and writing into the container won't work
	// We need host networking because the Tanzu installer will try to connect to a kind cluster it boots up
	cmd := exec.NewCommand(`docker run --rm \
		-v {{.SharedVolume}}:/root \
		-v /var/run/docker.sock:/var/run/docker.sock \
		-v /tmp:/tmp \
		-e HOME={{.Home}} \
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
		t.installAzureStoragePreview,
		t.ensureWorkDir,
		t.ensureStorageContainer,
		t.restoreInstallerState,
		t.prepareCLI,
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
	return azure.Login(t.azureCredentials)
}

// loginToContainerRegistry we use a private container registry to make the Tanzu CLI available in CI.
func (t *TanzuDriver) loginToContainerRegistry() error {
	log.Println("Logging in to container registry")
	// the Azure CLI image we use does not have a Docker client installed thus we extract a token here ...
	jsonResp, err := azure.Cmd("acr", "login", "--name", t.acrName, "--expose-token").
		StdoutOnly().WithoutStreaming().Output()
	if err != nil {
		return err
	}

	var loginDetails struct {
		AccessToken string `json:"accessToken"`
		LoginServer string `json:"loginServer"`
	}
	if err := json.Unmarshal([]byte(jsonResp), &loginDetails); err != nil {
		return err
	}
	// ... and do a manual docker login with the extracted token in the context of the CI image/your local dev machine instead
	return exec.NewCommand("docker login -u 00000000-0000-0000-0000-000000000000 -p {{.Token}} {{.Registry}}").AsTemplate(
		map[string]interface{}{
			"Token":    loginDetails.AccessToken,
			"Registry": loginDetails.LoginServer,
		},
	).WithoutStreaming().Run()
}

func (t *TanzuDriver) installAzureStoragePreview() error {
	log.Println("Installing Azure storage-preview extension")
	return azure.Cmd("extension add --name storage-preview -y").Run()
}

// ensureResourceGroup checks for the existence of an Azure resource group (which we name unless overridden after the
// cluster we want to deploy)
func (t *TanzuDriver) ensureResourceGroup() (bool, error) {
	exists, err := azure.Cmd("group", "exists", "--name", t.plan.Tanzu.ResourceGroup).
		WithoutStreaming().OutputContainsAny("true")
	if err != nil || exists {
		return false, err
	}
	log.Println("Creating Azure resource group")
	// https://learn.microsoft.com/en-us/cli/azure/group?view=azure-cli-latest#az-group-create
	err = azure.Cmd("group", "create",
		"-l", t.plan.Tanzu.Location,
		"--name", t.plan.Tanzu.ResourceGroup,
		"--tags", strings.Join(toList(elasticTags), " "),
	).WithoutStreaming().Run()
	return true, err
}

func (t *TanzuDriver) deleteResourceGroup() error {
	log.Println("Deleting Azure resource group")
	return azure.Cmd("group", "delete", "--name", t.plan.Tanzu.ResourceGroup, "-y").
		Run()
}

func (t *TanzuDriver) storageContainerExists() (bool, error) {
	return azure.ExistsCmd(azure.Cmd("storage", "container", "exists",
		"--account-name", t.azureStorageAccount, "--name", t.plan.ClusterName, "--auth", "login"))
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
	return azure.Cmd("storage", "container", "create",
		"--account-name", t.azureStorageAccount, "--name", t.plan.ClusterName, "--auth", "login").
		WithoutStreaming().Run()
}

func (t TanzuDriver) deleteStorageContainer() error {
	log.Println("Deleting Azure storage container")
	return azure.Cmd("storage", "container", "delete",
		"--account-name", t.azureStorageAccount, "--name", t.plan.ClusterName, "--auth", "login").
		WithoutStreaming().Run()
}

func (t *TanzuDriver) persistInstallerState() error {
	log.Println("Persisting installer state to Azure storage container")
	return azure.Cmd("storage", "azcopy", "blob", "upload", "--recursive",
		"-c", t.plan.ClusterName, "--account-name", t.azureStorageAccount,
		"-s", fmt.Sprintf("'%s/*'", t.installerStateDirBasename)).
		WithoutStreaming().Run()
}

func (t *TanzuDriver) restoreInstallerState() error {
	log.Println("Restoring installer state from storage container if any")
	return azure.Cmd("storage", "azcopy", "blob", "download", "--recursive",
		"-c", t.plan.ClusterName, "--account-name", t.azureStorageAccount,
		"-s", "'*'", "-d", t.installerStateDirBasename).
		WithoutStreaming().Run()
}

var _ Driver = &TanzuDriver{}
