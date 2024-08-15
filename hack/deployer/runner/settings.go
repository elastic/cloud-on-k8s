// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package runner

import (
	"os"

	"gopkg.in/yaml.v3"
)

// Plans encapsulates list of plans, expected to map to a file
type Plans struct {
	Plans []Plan
}

// Plan encapsulates information needed to provision a cluster
type Plan struct {
	Id                string `yaml:"id"` //nolint:revive
	Operation         string `yaml:"operation"`
	ClusterName       string `yaml:"clusterName"`
	ClientVersion     string `yaml:"clientVersion"`
	ClientBuildDefDir string `yaml:"clientBuildDefDir"`
	Provider          string `yaml:"provider"`
	KubernetesVersion string `yaml:"kubernetesVersion"`
	MachineType       string `yaml:"machineType"`
	// Abbreviations not all-caps to allow merging with mergo in  `merge` as mergo does not understand struct tags and
	// we use lowercase in the YAML
	Gke                     *GKESettings  `yaml:"gke,omitempty"`
	Aks                     *AKSSettings  `yaml:"aks,omitempty"`
	Ocp                     *OCPSettings  `yaml:"ocp,omitempty"`
	Eks                     *EKSSettings  `yaml:"eks,omitempty"`
	Kind                    *KindSettings `yaml:"kind,omitempty"`
	ServiceAccount          bool          `yaml:"serviceAccount"`
	EnforceSecurityPolicies bool          `yaml:"enforceSecurityPolicies"`
	DiskSetup               string        `yaml:"diskSetup"`
}

// GKESettings encapsulates settings specific to GKE
type GKESettings struct {
	GCloudProject    string `yaml:"gCloudProject"`
	Region           string `yaml:"region"`
	LocalSsdCount    int    `yaml:"localSsdCount"`
	NodeCountPerZone int    `yaml:"nodeCountPerZone"`
	GcpScopes        string `yaml:"gcpScopes"`
	ClusterIPv4CIDR  string `yaml:"clusterIpv4Cidr"`
	ServicesIPv4CIDR string `yaml:"servicesIpv4Cidr"`
	Private          bool   `yaml:"private"`
	NetworkPolicy    bool   `yaml:"networkPolicy"`
	Autopilot        bool   `yaml:"autopilot"`
}

// AKSSettings encapsulates settings specific to AKS
type AKSSettings struct {
	ResourceGroup string `yaml:"resourceGroup"`
	Location      string `yaml:"location"`
	Zones         string `yaml:"zones"`
	NodeCount     int    `yaml:"nodeCount"`
}

// OCPSettings encapsulates settings specific to OCP on GCloud
type OCPSettings struct {
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

// RunConfig encapsulates Id used to choose a plan and a map of overrides to apply to the plan, expected to map to a file
type RunConfig struct {
	Id        string                 `yaml:"id"` //nolint:revive
	Overrides map[string]interface{} `yaml:"overrides"`
}

func ParseFiles(plansFile, runConfigFile string) (Plans, RunConfig, error) {
	yml, err := os.ReadFile(plansFile)
	if err != nil {
		return Plans{}, RunConfig{}, err
	}

	var plans Plans
	err = yaml.Unmarshal(yml, &plans)
	if err != nil {
		return Plans{}, RunConfig{}, err
	}

	yml, err = os.ReadFile(runConfigFile)
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
