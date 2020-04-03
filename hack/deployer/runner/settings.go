// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package runner

import (
	"io/ioutil"

	"gopkg.in/yaml.v3"
)

// Plans encapsulates list of plans, expected to map to a file
type Plans struct {
	Plans []Plan
}

// Plan encapsulates information needed to provision a cluster
type Plan struct {
	Id                string       `yaml:"id"` //nolint
	Operation         string       `yaml:"operation"`
	ClusterName       string       `yaml:"clusterName"`
	Provider          string       `yaml:"provider"`
	KubernetesVersion string       `yaml:"kubernetesVersion"`
	MachineType       string       `yaml:"machineType"`
	GKE               *GKESettings `yaml:"gke,omitempty"`
	AKS               *AKSSettings `yaml:"aks,omitempty"`
	OCP               *OCPSettings `yaml:"ocp,omitempty"`
	VaultInfo         *VaultInfo   `yaml:"vaultInfo,omitempty"`
	ServiceAccount    bool         `yaml:"serviceAccount"`
	Psp               bool         `yaml:"psp"`
}

type VaultInfo struct {
	Address     string `yaml:"address"`
	RoleId      string `yaml:"roleId"`   //nolint
	SecretId    string `yaml:"secretId"` //nolint
	Token       string `yaml:"token"`
	ClientToken string `yaml:"clientToken"`
}

// GKESettings encapsulates settings specific to GKE
type GKESettings struct {
	GCloudProject    string `yaml:"gCloudProject"`
	Region           string `yaml:"region"`
	AdminUsername    string `yaml:"adminUsername"`
	LocalSSDCount    int    `yaml:"localSSDCount"`
	NodeCountPerZone int    `yaml:"nodeCountPerZone"`
	GcpScopes        string `yaml:"gcpScopes"`
	ClusterIPv4CIDR  string `yaml:"clusterIpv4Cidr"`
	ServicesIPv4CIDR string `yaml:"servicesIpv4Cidr"`
}

// AKSSettings encapsulates settings specific to AKS
type AKSSettings struct {
	ResourceGroup string `yaml:"resourceGroup"`
	Location      string `yaml:"location"`
	ACRName       string `yaml:"acrName"`
	NodeCount     int    `yaml:"nodeCount"`
}

// GKESettings encapsulates settings specific to GKE
type OCPSettings struct {
	BaseDomain                 string `yaml:"baseDomain"`
	GCloudProject              string `yaml:"gCloudProject"`
	Region                     string `yaml:"region"`
	AdminUsername              string `yaml:"adminUsername"`
	WorkDir                    string `yaml:"workDir"`
	PullSecret                 string `yaml:"pullSecret"`
	OverwriteDefaultKubeconfig bool   `yaml:"overwriteDefaultKubeconfig"`
	LocalSSDCount              int    `yaml:"localSSDCount"`
	NodeCount                  int    `yaml:"nodeCount"`
}

// RunConfig encapsulates Id used to choose a plan and a map of overrides to apply to the plan, expected to map to a file
type RunConfig struct {
	Id        string                 `yaml:"id"` //nolint
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
