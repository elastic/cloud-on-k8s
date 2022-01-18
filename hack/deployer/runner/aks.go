// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package runner

import (
	"fmt"
	"log"
)

func init() {
	drivers[AksDriverID] = &AksDriverFactory{}
}

const (
	AksDriverID                    = "aks"
	AksVaultPath                   = "secret/devops-ci/cloud-on-k8s/ci-azr-k8s-operator"
	AksResourceGroupVaultFieldName = "resource-group"
	DefaultAksRunConfigTemplate    = `id: aks-dev
overrides:
  clusterName: %s-dev-cluster
  aks:
    resourceGroup: %s
`
)

type AksDriverFactory struct {
}

type AksDriver struct {
	plan        Plan
	ctx         map[string]interface{}
	vaultClient *VaultClient
}

func (gdf *AksDriverFactory) Create(plan Plan) (Driver, error) {
	var vaultClient *VaultClient
	if plan.VaultInfo != nil {
		var err error
		vaultClient, err = NewClient(*plan.VaultInfo)
		if err != nil {
			return nil, err
		}

		if plan.Aks.ResourceGroup == "" {
			resourceGroup, err := vaultClient.Get(AksVaultPath, AksResourceGroupVaultFieldName)
			if err != nil {
				return nil, err
			}
			plan.Aks.ResourceGroup = resourceGroup
		}
	}

	return &AksDriver{
		plan: plan,
		ctx: map[string]interface{}{
			"ResourceGroup":     plan.Aks.ResourceGroup,
			"ClusterName":       plan.ClusterName,
			"NodeCount":         plan.Aks.NodeCount,
			"MachineType":       plan.MachineType,
			"KubernetesVersion": plan.KubernetesVersion,
			"Location":          plan.Aks.Location,
			"Zones":             plan.Aks.Zones,
		},
		vaultClient: vaultClient,
	}, nil
}

func (d *AksDriver) Execute() error {
	if err := d.auth(); err != nil {
		return err
	}

	exists, err := d.clusterExists()
	if err != nil {
		return err
	}

	switch d.plan.Operation {
	case DeleteAction:
		if exists {
			if err := d.delete(); err != nil {
				return err
			}
		} else {
			log.Printf("not deleting as cluster doesn't exist")
		}
	case CreateAction:
		if exists {
			log.Printf("not creating as cluster exists")
		} else if err := d.create(); err != nil {
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
		return fmt.Errorf("unknown operation %s", d.plan.Operation)
	}

	return nil
}

func (d *AksDriver) auth() error {
	if d.plan.ServiceAccount {
		log.Print("Authenticating as service account...")
		credentials, err := newAzureCredentials(d.vaultClient)
		if err != nil {
			return err
		}
		return azureLogin(credentials)
	}

	log.Print("Authenticating as user...")
	return NewCommand("az login").Run()
}

func (d *AksDriver) clusterExists() (bool, error) {
	log.Print("Checking if cluster exists...")

	cmd := "az aks show --name {{.ClusterName}} --resource-group {{.ResourceGroup}}"
	contains, err := NewCommand(cmd).AsTemplate(d.ctx).WithoutStreaming().OutputContainsAny("not be found", "was not found")
	if contains {
		return false, nil
	}

	return err == nil, err
}

func (d *AksDriver) create() error {
	log.Print("Creating cluster...")

	servicePrincipal := ""
	if d.plan.ServiceAccount {
		// our service principal doesn't have permissions to create a service principal for aks cluster
		// instead, we reuse the current service principal as the one for aks cluster
		secrets, err := d.vaultClient.GetMany(AksVaultPath, "appId", "password")
		if err != nil {
			return err
		}
		servicePrincipal = fmt.Sprintf(" --service-principal %s --client-secret %s", secrets[0], secrets[1])
	}

	cmd := `az aks create --resource-group {{.ResourceGroup}} --name {{.ClusterName}} --location {{.Location}} ` +
		`--node-count {{.NodeCount}} --node-vm-size {{.MachineType}} --kubernetes-version {{.KubernetesVersion}} ` +
		`--node-osdisk-size 30 --enable-addons http_application_routing --output none --generate-ssh-keys --zones {{.Zones}}` + servicePrincipal

	return NewCommand(cmd).AsTemplate(d.ctx).Run()
}

func (d *AksDriver) GetCredentials() error {
	log.Print("Getting credentials...")
	cmd := `az aks get-credentials --overwrite-existing --resource-group {{.ResourceGroup}} --name {{.ClusterName}}`
	return NewCommand(cmd).AsTemplate(d.ctx).Run()
}

func (d *AksDriver) delete() error {
	log.Print("Deleting cluster...")
	cmd := "az aks delete --yes --name {{.ClusterName}} --resource-group {{.ResourceGroup}}"
	return NewCommand(cmd).AsTemplate(d.ctx).Run()
}
