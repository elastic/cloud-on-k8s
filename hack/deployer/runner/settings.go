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
	Gke                     *GKESettings    `yaml:"gke,omitempty"`
	Aks                     *AKSSettings    `yaml:"aks,omitempty"`
	Ocp                     *OCPSettings    `yaml:"ocp,omitempty"`
	Eks                     *EKSSettings    `yaml:"eks,omitempty"`
	Kind                    *KindSettings   `yaml:"kind,omitempty"`
	K3d                     *K3dSettings    `yaml:"k3d,omitempty"`
	Bucket                  *BucketSettings `yaml:"bucket,omitempty"`
	ServiceAccount          bool            `yaml:"serviceAccount"`
	EnforceSecurityPolicies bool            `yaml:"enforceSecurityPolicies"`
	DiskSetup               string          `yaml:"diskSetup"`
}

// BucketSettings encapsulates settings for cloud storage bucket provisioning.
type BucketSettings struct {
	// Name is the bucket name. Supports template variables (e.g. "{{ .ClusterName }}-development").
	Name string `yaml:"name"`
	// Region is the cloud region for the bucket. For cloud providers (GKE, EKS, AKS) this is
	// overridden by the provider-specific region. For local clusters (Kind, K3D) it defaults to us-central1.
	Region string `yaml:"region,omitempty"`
	// StorageClass is the cloud storage class (e.g. "standard" for GCS, "STANDARD" for S3).
	StorageClass string `yaml:"storageClass"`
	// Secret is the K8s Secret where the bucket credentials will be stored.
	Secret BucketSecretSettings `yaml:"secret"`
	// S3 holds AWS S3-specific configuration. Only used when the provider is EKS.
	S3 *S3BucketSettings `yaml:"s3,omitempty"`
}

// S3BucketSettings holds AWS-specific configuration for S3 bucket provisioning.
type S3BucketSettings struct {
	// IamUserPath is the IAM path under which storage users should be created.
	// "Iam" (not "IAM") so mergo maps the YAML key "iamUserPath" correctly.
	IamUserPath string `yaml:"iamUserPath"`
	// ManagedPolicyARN is the ARN of the pre-existing managed policy to attach to IAM users.
	ManagedPolicyARN string `yaml:"managedPolicyARN"`
}

// BucketSecretSettings defines the Kubernetes Secret where bucket credentials are stored.
type BucketSecretSettings struct {
	Name      string `yaml:"name"`
	Namespace string `yaml:"namespace"`
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

type K3dSettings struct {
	ClientImage string `yaml:"clientImage"`
	NodeImage   string `yaml:"nodeImage"`
}

// RunConfig encapsulates Id used to choose a plan and a map of overrides to apply to the plan, expected to map to a file
type RunConfig struct {
	Id        string         `yaml:"id"` //nolint:revive
	Overrides map[string]any `yaml:"overrides"`
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
