// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package runner

import (
	_ "embed"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/vault"
)

//go:embed plans.yml
var plans string

// Plans encapsulates list of plans, expected to map to a file
type Plans struct {
	Plans []Plan
}

// Plan encapsulates information needed to provision a cluster
type Plan struct {
	Id                string `yaml:"id"` //nolint:revive
	Operation         string `yaml:"operation"`
	ClusterName       string `yaml:"clusterName"`
	ClientVersion     string `yaml:"clientVersion,omitempty"`
	ClientBuildDefDir string `yaml:"clientBuildDefDir"`
	Provider          string `yaml:"provider"`
	KubernetesVersion string `yaml:"kubernetesVersion"`
	MachineType       string `yaml:"machineType"`
	// Abbreviations not all-caps to allow merging with mergo in  `merge` as mergo does not understand struct tags and
	// we use lowercase in the YAML
	Gke                     *GKESettings   `yaml:"gke,omitempty"`
	Aks                     *AKSSettings   `yaml:"aks,omitempty"`
	Ocp                     *OCPSettings   `yaml:"ocp,omitempty"`
	Eks                     *EKSSettings   `yaml:"eks,omitempty"`
	Kind                    *KindSettings  `yaml:"kind,omitempty"`
	Tanzu                   *TanzuSettings `yaml:"tanzu,omitempty"`
	ServiceAccount          bool           `yaml:"serviceAccount"`
	EnforceSecurityPolicies bool           `yaml:"enforceSecurityPolicies"`
	DiskSetup               string         `yaml:"diskSetup"`
}

// GKESettings encapsulates settings specific to GKE
type GKESettings struct {
	GCloudProject    string `yaml:"gCloudProject"`
	Region           string `yaml:"region"`
	LocalSsdCount    int    `yaml:"localSsdCount"`
	NodeCountPerZone int    `yaml:"nodeCountPerZone"`
	GcpScopes        string `yaml:"gcpScopes"`
	ClusterIPv4CIDR  string `yaml:"clusterIpv4Cidr,omitempty"`
	ServicesIPv4CIDR string `yaml:"servicesIpv4Cidr,omitempty"`
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

type TanzuSettings struct {
	AKSSettings    `yaml:",inline"`
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
	var yml []byte
	if plansFile == "" {
		yml = []byte(plans)
	} else {
		var err error
		yml, err = os.ReadFile(plansFile)
		if err != nil {
			return Plans{}, RunConfig{}, err
		}
	}

	var plans Plans
	err := yaml.Unmarshal(yml, &plans)
	if err != nil {
		return Plans{}, RunConfig{}, err
	}

	runConfig := RunConfig{}
	if runConfigFile != "" {
		yml, err = os.ReadFile(runConfigFile)
		if err != nil {
			return Plans{}, RunConfig{}, err
		}
		var runConfig RunConfig
		err = yaml.Unmarshal(yml, &runConfig)
		if err != nil {
			return Plans{}, RunConfig{}, err
		}
	}

	return plans, runConfig, nil
}

// order: -id, deployer-config.yml, env
func GetPlan(plans []Plan, config RunConfig, clientBuildDefDir, id string) (Plan, error) {
	envID, fromEnv := os.LookupEnv("E2E_PROVIDER")

	if id != "" && config.Id == "" {
		config.Id = id
	} else if fromEnv {
		config.Id = envID + "-ci"
	}

	plan, err := choosePlan(plans, config.Id)
	if err != nil {
		return Plan{}, err
	}

	if config.Overrides == nil {
		config.Overrides = map[string]interface{}{}
	}

	// default that should be set in the env, otherwise the deployer will fail
	config.Overrides["vaultInfo"] = map[string]interface{}{
		"address":   os.Getenv(EnvVarVaultAddr),
		"vaultInfo": vault.RootPath(),
	}

	// optional cluster name
	if val, ok := os.LookupEnv(EnvVarClusterName); ok {
		config.Overrides["clusterName"] = val
	}

	// automatically set gcloud project for gke and ocp
	if plan.Provider == "gke" || plan.Provider == "ocp" {
		gCloudProject := DefaultGCloudProject
		if val, ok := os.LookupEnv(EnvVargGloudProject); ok {
			gCloudProject = val
		}
		config.Overrides[plan.Provider] = map[string]interface{}{
			"gCloudProject": gCloudProject,
		}
	}

	if fromEnv {
		addOverridesFromEnv(&config)
	}

	plan, err = merge(plan, config.Overrides)
	if err != nil {
		return Plan{}, err
	}

	// allows plans and runConfigs to set this value but use a default if not set
	if plan.ClientBuildDefDir == "" {
		plan.ClientBuildDefDir = clientBuildDefDir
	}

	// Print plan for debug purposes
	bytes, _ := yaml.Marshal(plan)
	fmt.Println("--- deployer plan ---")
	fmt.Println(string(bytes))
	fmt.Println("--- deployer plan ---")

	return plan, nil
}

const (
	EnvVarClusterName       = "CLUSTER_NAME"
	EnvVarDeployerOperation = "DEPLOYER_operation"

	EnvVarVaultAddr     = "VAULT_ADDR"
	EnvVargGloudProject = "GCLOUD_PROJECT"

	EnvVarDeployerPrefix = "DEPLOYER_"

	DefaultGCloudProject = "elastic-cloud-dev"
)

// addOverridesFromEnv discovers environment variables prefixed with DEPLOYER_.
// '_' separator is used to specify a new level and settings must be camelCased.
// DEPLOYER_kind_nodeImage=xyz is transformed in
//
//	kind:
//	  nodeImage: xyz
func addOverridesFromEnv(config *RunConfig) RunConfig {
	for _, env := range os.Environ() {
		if strings.HasPrefix(env, EnvVarDeployerPrefix) {
			kv := strings.Split(strings.ReplaceAll(env, EnvVarDeployerPrefix, ""), "=")
			keyElems := strings.Split(kv[0], "_")
			config.Overrides[keyElems[0]] = reccursiveVars(map[string]interface{}{}, keyElems[1:], kv[1])
		}
	}

	return *config
}

func reccursiveVars(vals interface{}, keyElems []string, val string) interface{} {
	if len(keyElems) == 0 {
		return val
	}
	if len(keyElems) == 1 {
		//nolint:forcetypeassert
		vals.(map[string]interface{})[keyElems[0]] = val
		return vals
	}
	//nolint:forcetypeassert
	vals.(map[string]interface{})[keyElems[0]] = reccursiveVars(map[string]interface{}{}, keyElems[1:], val)
	return vals
}
