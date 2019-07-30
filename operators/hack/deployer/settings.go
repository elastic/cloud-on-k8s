// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package main

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

	VaultAddress  string `yaml:"vaultAddress"`
	VaultRoleId   string `yaml:"vaultRoleId"`
	VaultSecretId string `yaml:"vaultSecretId"`
}

// GkeSettings encapculates settings specific to GKE
type GkeSettings struct {
	GCloudProject    string `yaml:"gcloudProject"`
	Region           string `yaml:"region"`
	AdminUsername    string `yaml:"adminUsername"`
	LocalSsdCount    int64  `yaml:"localSsdCount"`
	NodeCountPerZone int64  `yaml:"nodeCountPerZone"`
	GcpScopes        string `yaml:"gcpScopes"`
}

// RunConfig encapsulates Id used to choose a plan and a map of overrides to apply to the plan, expected to map to a file
type RunConfig struct {
	Id        string                 `yaml:"id"`
	Overrides map[string]interface{} `yaml:"overrides"`
}
