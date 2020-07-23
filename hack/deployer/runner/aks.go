// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package runner

import (
	"fmt"
	"log"
)

func init() {
	drivers[AKSDriverID] = &AKSDriverFactory{}
}

const (
	AKSDriverID                    = "aks"
	AKSVaultPath                   = "secret/devops-ci/cloud-on-k8s/ci-azr-k8s-operator"
	AKSResourceGroupVaultFieldName = "resource-group"
	AKSConfigFileName              = "deployer-config-aks.yml"
	DefaultAKSRunConfigTemplate    = `id: aks-dev
overrides:
  clusterName: %s-dev-cluster
  aks:
    resourceGroup: %s
`
)

type AKSDriverFactory struct {
}

type AKSDriver struct {
	plan        Plan
	ctx         map[string]interface{}
	vaultClient *VaultClient
}

func (gdf *AKSDriverFactory) Create(plan Plan) (Driver, error) {
	var vaultClient *VaultClient
	if plan.VaultInfo != nil {
		var err error
		vaultClient, err = NewClient(*plan.VaultInfo)
		if err != nil {
			return nil, err
		}

		if plan.AKS.ResourceGroup == "" {
			resourceGroup, err := vaultClient.Get(AKSVaultPath, AKSResourceGroupVaultFieldName)
			if err != nil {
				return nil, err
			}
			plan.AKS.ResourceGroup = resourceGroup
		}
	}

	return &AKSDriver{
		plan: plan,
		ctx: map[string]interface{}{
			"ResourceGroup":     plan.AKS.ResourceGroup,
			"ClusterName":       plan.ClusterName,
			"NodeCount":         plan.AKS.NodeCount,
			"MachineType":       plan.MachineType,
			"KubernetesVersion": plan.KubernetesVersion,
			"Location":          plan.AKS.Location,
		},
		vaultClient: vaultClient,
	}, nil
}

func (d *AKSDriver) Execute() error {
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

		if err := createStorageClass(NoProvisioner); err != nil {
			return err
		}

		if err := NewCommand(d.plan.AKS.DiskSetup).Run(); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown operation %s", d.plan.Operation)
	}

	return nil
}

func (d *AKSDriver) auth() error {
	if d.plan.ServiceAccount {
		log.Print("Authenticating as service account...")

		secrets, err := d.vaultClient.GetMany(AKSVaultPath, "appId", "password", "tenant")
		if err != nil {
			return err
		}
		appID, tenantSecret, tenantID := secrets[0], secrets[1], secrets[2]

		cmd := "az login --service-principal -u {{.AppId}} -p {{.TenantSecret}} --tenant {{.TenantId}}"
		return NewCommand(cmd).
			AsTemplate(map[string]interface{}{
				"AppId":        appID,
				"TenantSecret": tenantSecret,
				"TenantId":     tenantID,
			}).
			WithoutStreaming().
			Run()
	}

	log.Print("Authenticating as user...")
	return NewCommand("az login").Run()
}

func (d *AKSDriver) clusterExists() (bool, error) {
	log.Print("Checking if cluster exists...")

	cmd := "az aks show --name {{.ClusterName}} --resource-group {{.ResourceGroup}}"
	contains, err := NewCommand(cmd).AsTemplate(d.ctx).WithoutStreaming().OutputContainsAny("not be found", "was not found")
	if contains {
		return false, nil
	}

	return err == nil, err
}

func (d *AKSDriver) create() error {
	log.Print("Creating cluster...")

	servicePrincipal := ""
	if d.plan.ServiceAccount {
		// our service principal doesn't have permissions to create a service principal for aks cluster
		// instead, we reuse the current service principal as the one for aks cluster
		secrets, err := d.vaultClient.GetMany(AKSVaultPath, "appId", "password")
		if err != nil {
			return err
		}
		servicePrincipal = fmt.Sprintf(" --service-principal %s --client-secret %s", secrets[0], secrets[1])
	}

	cmd := `az aks create --resource-group {{.ResourceGroup}} --name {{.ClusterName}} --location {{.Location}} ` +
		`--node-count {{.NodeCount}} --node-vm-size {{.MachineType}} --kubernetes-version {{.KubernetesVersion}} ` +
		`--node-osdisk-size 30 --enable-addons http_application_routing --generate-ssh-keys` + servicePrincipal

	if err := NewCommand(cmd).AsTemplate(d.ctx).Run(); err != nil {
		return err
	}

	return nil
}

func (d *AKSDriver) GetCredentials() error {
	log.Print("Getting credentials...")
	cmd := `az aks get-credentials --overwrite-existing --resource-group {{.ResourceGroup}} --name {{.ClusterName}}`
	return NewCommand(cmd).AsTemplate(d.ctx).Run()
}

func (d *AKSDriver) delete() error {
	log.Print("Deleting cluster...")
	cmd := "az aks delete --yes --name {{.ClusterName}} --resource-group {{.ResourceGroup}}"
	return NewCommand(cmd).AsTemplate(d.ctx).Run()
}
