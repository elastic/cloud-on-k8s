// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package runner

import (
	"io/ioutil"

	"github.com/ghodss/yaml"
)

// Plans encapsulates list of plans, expected to map to a file
type Plans struct {
	Plans []Plan
}

// Plan encapsulates information needed to provision a cluster
type Plan struct {
	Id                string `yaml:"id"`
	Operation         string `yaml:"operation"`
	ClusterName       string `yaml:"clusterName"`
	Provider          string `yaml:"provider"`
	KubernetesVersion string `yaml:"kubernetesVersion"`
	MachineType       string `yaml:"machineType"`
	ServiceAccount    bool   `yaml:"serviceAccount"`

	Psp      bool `yaml:"psp"`
	VmMapMax bool `yaml:"vmMapMax"`

	Gke *GkeSettings `yaml:"gke,omitempty"`
	Aks *AksSettings `yaml:"aks,omitempty"`

	VaultInfo *VaultInfo `yaml:"vaultInfo,omitempty"`
}

type VaultInfo struct {
	Address  string `yaml:"address"`
	RoleId   string `yaml:"roleId"`
	SecretId string `yaml:"secretId"`
	Token    string `yaml:"token"`
}

// GkeSettings encapsulates settings specific to GKE
type GkeSettings struct {
	GCloudProject    string `yaml:"gCloudProject"`
	Region           string `yaml:"region"`
	AdminUsername    string `yaml:"adminUsername"`
	LocalSsdCount    int64  `yaml:"localSsdCount"`
	NodeCountPerZone int64  `yaml:"nodeCountPerZone"`
	GcpScopes        string `yaml:"gcpScopes"`
}

// AksSettings encapsulates settings specific to AKS
type AksSettings struct {
	ResourceGroup string `yaml:"resourceGroup"`
	AcrName       string `yaml:"acrName"`
	NodeCount     int64  `yaml:"nodeCount"`
}

// RunConfig encapsulates Id used to choose a plan and a map of overrides to apply to the plan, expected to map to a file
type RunConfig struct {
	Id        string                 `yaml:"id"`
	Overrides map[string]interface{} `yaml:"overrides"`
}

func ParseFiles(plansFile, runConfigFile string) (Plans, RunConfig, error) {
	yml, err := ioutil.ReadFile(plansFile)
	if err != nil {
		return Plans{}, RunConfig{}, err
	}

	var plans Plans
	err = yaml.Unmarshal(yml, &plans)
	if err != nil {
		return Plans{}, RunConfig{}, err
	}

	yml, err = ioutil.ReadFile(runConfigFile)
	if err != nil {
		return Plans{}, RunConfig{}, err
	}

	var runConfig RunConfig
	err = yaml.Unmarshal(yml, &runConfig)
	if err != nil {
		return Plans{}, RunConfig{}, err
	}

	return plans, runConfig, nil
}