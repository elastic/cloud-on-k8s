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
	Id                string       `yaml:"id"` // nolint
	Operation         string       `yaml:"operation"`
	ClusterName       string       `yaml:"clusterName"`
	Provider          string       `yaml:"provider"`
	KubernetesVersion string       `yaml:"kubernetesVersion"`
	MachineType       string       `yaml:"machineType"`
	Gke               *GkeSettings `yaml:"gke,omitempty"`
	Aks               *AksSettings `yaml:"aks,omitempty"`
	Ocp               *OcpSettings `yaml:"ocp,omitempty"`
	EKS               *EKSSettings `yaml:"eks,omitempty"`
	VaultInfo         *VaultInfo   `yaml:"vaultInfo,omitempty"`
	ServiceAccount    bool         `yaml:"serviceAccount"`
	Psp               bool         `yaml:"psp"`
}

type VaultInfo struct {
	Address     string `yaml:"address"`
	RoleId      string `yaml:"roleId"`   // nolint
	SecretId    string `yaml:"secretId"` // nolint
	Token       string `yaml:"token"`
	ClientToken string `yaml:"clientToken"`
}

// GkeSettings encapsulates settings specific to GKE
type GkeSettings struct {
	GCloudProject    string `yaml:"gCloudProject"`
	Region           string `yaml:"region"`
	AdminUsername    string `yaml:"adminUsername"`
	LocalSsdCount    int    `yaml:"localSsdCount"`
	NodeCountPerZone int    `yaml:"nodeCountPerZone"`
	GcpScopes        string `yaml:"gcpScopes"`
	ClusterIPv4CIDR  string `yaml:"clusterIpv4Cidr"`
	ServicesIPv4CIDR string `yaml:"servicesIpv4Cidr"`
	DiskSetup        string `yaml:"diskSetup"`
}

// AksSettings encapsulates settings specific to AKS
type AksSettings struct {
	ResourceGroup string `yaml:"resourceGroup"`
	Location      string `yaml:"location"`
	NodeCount     int    `yaml:"nodeCount"`
}

// OcpSettings encapsulates settings specific to OCP on GCloud
type OcpSettings struct {
	BaseDomain                 string `yaml:"baseDomain"`
	GCloudProject              string `yaml:"gCloudProject"`
	Region                     string `yaml:"region"`
	AdminUsername              string `yaml:"adminUsername"`
	WorkDir                    string `yaml:"workDir"`
	PullSecret                 string `yaml:"pullSecret"`
	LocalSsdCount              int    `yaml:"localSsdCount"`
	NodeCount                  int    `yaml:"nodeCount"`
	OverwriteDefaultKubeconfig bool   `yaml:"overwriteDefaultKubeconfig"`
	// UseNonDefaultCloudSDKPath, if true, sets $CLOUDSDK_CONFIG to a non-default value in order
	// to not tamper existing gcloud credentials.
	UseNonDefaultCloudSDKPath bool `yaml:"useNonDefaultCloudSdkPath"`
}

// EKSSettings are specific to Amazon EKS.
type EKSSettings struct {
	NodeAMI   string `yaml:"nodeAMI"`
	NodeCount int    `yaml:"nodeCount"`
	Region    string `yaml:"region"`
	WorkDir   string `yaml:"workDir"`
	DiskSetup string `yaml:"diskSetup"`
}

// RunConfig encapsulates Id used to choose a plan and a map of overrides to apply to the plan, expected to map to a file
type RunConfig struct {
	Id        string                 `yaml:"id"` // nolint
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
