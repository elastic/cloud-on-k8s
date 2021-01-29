// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package runner

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v2"
)

const (
	Ocp3DriverID = "ocp3"

	// Field names of the SSH keys for GCP stored in Vault
	Ocp3GCloudPrivateSSHKeyFieldName = "gcloud-ssh-private-key"
	Ocp3GCloudPublicSSHKeyFieldName  = "gcloud-ssh-public-key"

	// Ansible Docker image to manage OCP3 environments
	AnsibleDockerImage = "eu.gcr.io/elastic-cloud-dev/ansible:439897e"
	AnsibleUser        = "jenkins"
	// Ansible user home where some files (GCP credentials, Ansible vars and output) are mounted from the CI container
	AnsibleHomePath           = "/home/ansible"
	AnsibleVarsFilename       = "vars.yml"
	AnsibleOutputDirname      = "output"
	AnsibleKubeconfigFilename = "config.openshift"

	// Default OCP3 configuration for the k8s master
	MasterCount    = 1
	MasterInstance = "n1-standard-2"
)

var (
	// SharedVolumeName name shared by CI container and Ansible container
	SharedVolumeName = os.Getenv("SHARED_VOLUME_NAME")
)

func init() {
	drivers[Ocp3DriverID] = &Ocp3DriverFactory{}
}

type Ocp3DriverFactory struct {
}

type Ocp3Driver struct {
	plan Plan
	ctx  map[string]interface{}
}

func (Ocp3DriverFactory) Create(plan Plan) (Driver, error) {
	return &Ocp3Driver{
		plan: plan,
		ctx: map[string]interface{}{
			"ClusterName": plan.ClusterName,
		},
	}, nil
}

func (d Ocp3Driver) Execute() error {
	var err error

	if err := authToGCP(
		d.plan.VaultInfo, OcpVaultPath, OcpServiceAccountVaultFieldName,
		d.plan.ServiceAccount, true, d.plan.Ocp3.GCloudProject,
	); err != nil {
		return err
	}

	if err := writeGCloudSSHKey(*d.plan.VaultInfo); err != nil {
		return err
	}

	if err := d.writeAnsibleVarsFile(); err != nil {
		return err
	}

	switch d.plan.Operation {
	case DeleteAction:
		if err := d.runAnsibleDockerContainer("delete"); err != nil {
			return err
		}
	case CreateAction:
		if err := d.runAnsibleDockerContainer("create"); err != nil {
			return err
		}

		if err := d.GetCredentials(); err != nil {
			return err
		}

		if err := setupDisks(d.plan); err != nil {
			return err
		}

		if err := createStorageClass(); err != nil {
			return err
		}
	default:
		err = fmt.Errorf("unknown operation %s", d.plan.Operation)
	}

	return err
}

func writeGCloudSSHKey(vaultInfo VaultInfo) error {
	log.Printf("Setting GCP SSH keys...")
	sshDir := filepath.Join(os.Getenv("HOME"), ".ssh")
	_ = os.MkdirAll(sshDir, os.ModePerm)

	client, err := NewClient(vaultInfo)
	if err != nil {
		return err
	}

	keyFileName := filepath.Join(sshDir, "google_compute_engine")
	if err := client.ReadIntoFile(keyFileName, OcpVaultPath, Ocp3GCloudPrivateSSHKeyFieldName); err != nil {
		return err
	}
	pubKeyFileName := filepath.Join(sshDir, "google_compute_engine.pub")
	if err := client.ReadIntoFile(pubKeyFileName, OcpVaultPath, Ocp3GCloudPublicSSHKeyFieldName); err != nil {
		return err
	}

	return nil
}

func (d Ocp3Driver) writeAnsibleVarsFile() error {
	log.Printf("Setting Ansible vars...")

	vars := struct {
		Suffix         string   `yaml:"suffix"`
		MasterCount    int      `yaml:"master_count"`
		MasterInstance string   `yaml:"master_instance"`
		WorkerCount    int      `yaml:"worker_count"`
		WorkerInstance string   `yaml:"worker_instance"`
		AllowedSources []string `yaml:"allowed_sources"`
	}{
		Suffix:         d.plan.ClusterName,
		MasterCount:    MasterCount,
		MasterInstance: MasterInstance,
		WorkerCount:    d.plan.Ocp3.WorkerCount,
		WorkerInstance: d.plan.MachineType,
		AllowedSources: []string{"0.0.0.0/0"}, // TODO: get current IP to be more restrictive?
	}
	varsBytes, err := yaml.Marshal(vars)
	if err != nil {
		return err
	}
	varsFile := filepath.Join(os.Getenv("HOME"), AnsibleVarsFilename)
	/* #nosec */
	if err := ioutil.WriteFile(varsFile, varsBytes, 0644); err != nil {
		return fmt.Errorf("while writing Ansible variables file %w", err)
	}
	return nil
}

func (d Ocp3Driver) runAnsibleDockerContainer(action string) error {
	log.Printf("Creating OCP3 cluster with Ansible in Docker...")

	params := map[string]interface{}{
		"User":                AnsibleUser,
		"HomeVolumeName":      SharedVolumeName,
		"HomeVolumeMountPath": AnsibleHomePath,
		"GCloudCredsPath":     filepath.Join(AnsibleHomePath, GCPDir, ServiceAccountFilename),
		"GCloudSDKPath":       filepath.Join(AnsibleHomePath, GCPDir),
		"AnsibleVarsPath":     filepath.Join(AnsibleHomePath, AnsibleVarsFilename),
		"AnsibleOutputPath":   filepath.Join(AnsibleHomePath, AnsibleOutputDirname),
		"AnsibleAction":       action,
		"AnsibleDockerImage":  AnsibleDockerImage,
	}

	// CLOUDSDK_CONFIG env var is passed as-is to make gcloud sdk directory consistent between the host and the container
	return NewCommand(`docker run --rm \
		-e FORCED_GROUP_ID=1000 \
		-e FORCED_USER_ID=1000 \
		-e USER={{.User}} \
		-e USER_HOME={{.HomeVolumeMountPath}} \
		-v {{.HomeVolumeName}}:{{.HomeVolumeMountPath}} \
		-e CLOUDSDK_CONFIG \
		-e GOOGLE_APPLICATION_CREDENTIALS={{.GCloudCredsPath}} \
		-e VARS_FILE={{.AnsibleVarsPath}} \
		-e OUTPUT_DIR={{.AnsibleOutputPath}} \
		-e ACTION={{.AnsibleAction}} \
		-e CLUSTER_TYPE=openshift \
		{{.AnsibleDockerImage}}`).AsTemplate(params).Run()
}

func (Ocp3Driver) GetCredentials() error {
	kubeConfig := filepath.Join(os.Getenv("HOME"), AnsibleOutputDirname, AnsibleKubeconfigFilename)
	log.Printf("copying %s to ~/.kube/config", kubeConfig)
	if err := os.MkdirAll(filepath.Join(os.Getenv("HOME"), ".kube"), os.ModePerm); err != nil {
		return err
	}
	cmd := fmt.Sprintf("cp %s ~/.kube/config", kubeConfig)
	return NewCommand(cmd).WithoutStreaming().Run()
}
