// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package runner

import (
	"io/ioutil"
	"os"

	"gopkg.in/yaml.v3"
)

// SharedVolumeName name shared by CI container and Docker containers launched by deployer. This is the name of the volume
// valid outside of the CI Docker container, necessary to create other containers referencing the same volume.
// In local dev mode it is just the home dir as we are typically not running inside a container in the case.
func SharedVolumeName() string {
	if vol := os.Getenv("SHARED_VOLUME_NAME"); vol != "" {
		return vol
	}
	// use HOME for local dev mode
	return os.Getenv("HOME")
}

// Plans encapsulates list of plans, expected to map to a file
type Plans struct {
	Plans []Plan
}

// Plan encapsulates information needed to provision a cluster
type Plan struct {
	Id                string         `yaml:"id"` //nolint:revive
	Operation         string         `yaml:"operation"`
	ClusterName       string         `yaml:"clusterName"`
	ClientVersion     string         `yaml:"clientVersion"`
	ClientBuildDefDir string         `yaml:"clientBuildDefDir"`
	Provider          string         `yaml:"provider"`
	KubernetesVersion string         `yaml:"kubernetesVersion"`
	MachineType       string         `yaml:"machineType"`
	Gke               *GkeSettings   `yaml:"gke,omitempty"`
	Aks               *AksSettings   `yaml:"aks,omitempty"`
	Ocp               *OcpSettings   `yaml:"ocp,omitempty"`
	Ocp3              *Ocp3Settings  `yaml:"ocp3,omitempty"`
	EKS               *EKSSettings   `yaml:"eks,omitempty"`
	Kind              *KindSettings  `yaml:"kind,omitempty"`
	Tanzu             *TanzuSettings `yaml:"tanzu,omitempty"`
	VaultInfo         *VaultInfo     `yaml:"vaultInfo,omitempty"`
	ServiceAccount    bool           `yaml:"serviceAccount"`
	Psp               bool           `yaml:"psp"`
	DiskSetup         string         `yaml:"diskSetup"`
}

type VaultInfo struct {
	Address     string `yaml:"address"`
	RoleId      string `yaml:"roleId"`   //nolint:revive
	SecretId    string `yaml:"secretId"` //nolint:revive
	Token       string `yaml:"token"`
	ClientToken string `yaml:"clientToken"`
}

// GkeSettings encapsulates settings specific to GKE
type GkeSettings struct {
	GCloudProject    string `yaml:"gCloudProject"`
	Region           string `yaml:"region"`
	LocalSsdCount    int    `yaml:"localSsdCount"`
	NodeCountPerZone int    `yaml:"nodeCountPerZone"`
	GcpScopes        string `yaml:"gcpScopes"`
	ClusterIPv4CIDR  string `yaml:"clusterIpv4Cidr"`
	ServicesIPv4CIDR string `yaml:"servicesIpv4Cidr"`
	Private          bool   `yaml:"private"`
	NetworkPolicy    bool   `yaml:"networkPolicy"`
}

// AksSettings encapsulates settings specific to AKS
type AksSettings struct {
	ResourceGroup string `yaml:"resourceGroup"`
	Location      string `yaml:"location"`
	Zones         string `yaml:"zones"`
	NodeCount     int    `yaml:"nodeCount"`
}

// OcpSettings encapsulates settings specific to OCP on GCloud
type OcpSettings struct {
	BaseDomain    string `yaml:"baseDomain"`
	GCloudProject string `yaml:"gCloudProject"`
	Region        string `yaml:"region"`
	AdminUsername string `yaml:"adminUsername"`
	WorkDir       string `yaml:"workDir"`
	StickyWorkDir bool   `yaml:"stickyWorkDir"`
	PullSecret    string `yaml:"pullSecret"`
	LocalSsdCount int    `yaml:"localSsdCount"`
	NodeCount     int    `yaml:"nodeCount"`
}

// Ocp3Settings encapsulates settings specific to OCP3 on GCloud
type Ocp3Settings struct {
	GCloudProject string `yaml:"gCloudProject"`
	WorkerCount   int    `yaml:"workerCount"`
}

// EKSSettings are specific to Amazon EKS.
type EKSSettings struct {
	NodeAMI   string `yaml:"nodeAMI"`
	NodeCount int    `yaml:"nodeCount"`
	Region    string `yaml:"region"`
	WorkDir   string `yaml:"workDir"`
}

type KindSettings struct {
	NodeCount int    `yaml:"nodeCount"`
	NodeImage string `yaml:"nodeImage"`
	IPFamily  string `yaml:"ipFamily"`
}

type TanzuSettings struct {
	AksSettings    `yaml:",inline"`
	InstallerImage string `yaml:"installerImage"`
	WorkDir        string `yaml:"workDir"`
	SSHPubKey      string `yaml:"sshPubKey"`
}

// RunConfig encapsulates Id used to choose a plan and a map of overrides to apply to the plan, expected to map to a file
type RunConfig struct {
	Id        string                 `yaml:"id"` //nolint:revive
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
