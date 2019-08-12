// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package main

import (
	"fmt"
	"log"
)

func init() {
	drivers[AksDriverId] = &AksDriverFactory{}
}

const (
	AksDriverId                    = "aks"
	AksVaultPath                   = "secret/cloud-team/cloud-ci/ci-azr-k8s-operator"
	AksResourceGroupVaultFieldName = "resource-group"
	AksAcrNameVaultFieldName       = "acr-name"
)

type AksDriverFactory struct {
}

type AksDriver struct {
	plan        Plan
	ctx         map[string]interface{}
	vaultClient *Client
}

func (gdf *AksDriverFactory) Create(plan Plan) (Driver, error) {
	vaultClient, err := NewClient(*plan.VaultInfo)
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

	if plan.Aks.AcrName == "" {
		acrName, err := vaultClient.Get(AksVaultPath, AksAcrNameVaultFieldName)
		if err != nil {
			return nil, err
		}
		plan.Aks.AcrName = acrName
	}

	return &AksDriver{
		plan: plan,
		ctx: map[string]interface{}{
			"ResourceGroup":     plan.Aks.ResourceGroup,
			"ClusterName":       plan.ClusterName,
			"NodeCount":         plan.Aks.NodeCount,
			"MachineType":       plan.MachineType,
			"KubernetesVersion": plan.KubernetesVersion,
			"AcrName":           plan.Aks.AcrName,
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
	case "delete":
		log.Printf("Trying to delete GKE cluster with name: %s ...", d.ctx["ClusterName"])
		if exists {
			err = d.delete()
		} else {
			log.Printf("not deleting as cluster doesn't exist")
		}
	case "create":
		log.Printf("Trying to create AKS cluster using version: %s with name: %s ...", d.ctx["KubernetesVersion"], d.ctx["ClusterName"])
		if exists {
			log.Printf("not creating as cluster exists")
		} else {
			if err := d.create(); err != nil {
				return err
			}

			if !d.plan.ServiceAccount {
				// it's already set for the ServiceAccount
				if err := d.configureDocker(); err != nil {
					return err
				}
			}
		}

		if err := d.getCredentials(); err != nil {
			return err
		}
	default:
		err = fmt.Errorf("unknown operation %s", d.plan.Operation)
	}

	return nil
}

func (d *AksDriver) auth() error {
	if d.plan.ServiceAccount {
		log.Print("Authenticating as service account...")

		secrets, err := d.vaultClient.GetMany(AksVaultPath, "appId", "password", "tenant")
		if err != nil {
			return err
		}
		appId, tenantSecret, tenantId := secrets[0], secrets[1], secrets[2]

		cmd := "az login --service-principal -u {{.AppId}} -p {{.TenantSecret}} --tenant {{.TenantId}}"
		return NewCommand(cmd).
			AsTemplate(map[string]interface{}{
				"AppId":        appId,
				"TenantSecret": tenantSecret,
				"TenantId":     tenantId,
			}).
			WithoutStreaming().
			Run()
	} else {
		log.Print("Authenticating as user...")
		return NewCommand("az login").Run()
	}
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

	cmd := `az aks create --resource-group {{.ResourceGroup}} --name {{.ClusterName}} ` +
		`--node-count {{.NodeCount}} --node-vm-size {{.MachineType}} --kubernetes-version {{.KubernetesVersion}} ` +
		`--node-osdisk-size 30 --enable-addons http_application_routing,monitoring --generate-ssh-keys`
	if err := NewCommand(cmd).AsTemplate(d.ctx).Run(); err != nil {
		return err
	}

	return nil
}

func (d *AksDriver) configureDocker() error {
	log.Print("Configuring Docker...")
	if err := NewCommand("az acr login --name {{.AcrName}}").AsTemplate(d.ctx).Run(); err != nil {
		return err
	}

	cmd := `az aks show --resource-group {{.ResourceGroup}} --name {{.ClusterName}} --query "servicePrincipalProfile.clientId" --output tsv`
	clientIds, err := NewCommand(cmd).AsTemplate(d.ctx).StdoutOnly().OutputList()
	if err != nil {
		return err
	}

	cmd = `az acr show --resource-group {{.ResourceGroup}} --name {{.AcrName}} --query "id" --output tsv`
	acrIds, err := NewCommand(cmd).AsTemplate(d.ctx).StdoutOnly().OutputList()
	if err != nil {
		return err
	}

	return NewCommand(`az role assignment create --assignee {{.ClientId}} --role acrpull --scope {{.AcrId}}`).
		AsTemplate(map[string]interface{}{
			"ClientId": clientIds[0],
			"AcrId":    acrIds[0],
		}).
		Run()
}

func (d *AksDriver) getCredentials() error {
	log.Print("Getting credentials...")
	cmd := `az aks get-credentials --resource-group {{.ResourceGroup}} --name {{.ClusterName}}`
	return NewCommand(cmd).AsTemplate(d.ctx).Run()
}

func (d *AksDriver) delete() error {
	log.Print("Deleting cluster...")
	cmd := "az aks delete --yes --name {{.ClusterName}} --resource-group {{.ResourceGroup}}"
	return NewCommand(cmd).AsTemplate(d.ctx).Run()
}
