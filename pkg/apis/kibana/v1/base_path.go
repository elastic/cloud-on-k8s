// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1

import (
	"fmt"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	"github.com/elastic/go-ucfg"

	v1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/pod"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/settings"
)

const (
	// serverBasePathEnvName is the environment variable name that allows ibe to specify a path to mount Kibana at if you are running behind a proxy
	serverBasePathEnvName = "SERVER_BASEPATH"
	// serverRewriteBasePathEnvName is the environment variable name that specifies whether Kibana should rewrite requests that are prefixed with server.basePath
	serverRewriteBasePathEnvName = "SERVER_REWRITEBASEPATH"
)

// basePathConfig is used to get the base path from the Kibana configuration.
// +kubebuilder:object:generate=false
type basePathConfig struct {
	Server struct {
		RewriteBasePath *bool   `config:"rewriteBasePath"`
		BasePath        *string `config:"basePath"`
	} `config:"server"`
}

func (cfg basePathConfig) GetBasePath() string {
	if ptr.Deref(cfg.Server.RewriteBasePath, false) {
		return ptr.Deref(cfg.Server.BasePath, "")
	}

	return ""
}

func GetKibanaBasePath(kb Kibana) (string, error) {
	// We only support the case where both base path and rewrite base path are set in the ENV or the config
	// We will not support the case where base path is set in the ENV and rewrite base path is set in the config or vice versa
	kbBasePath, err := getKibanaBasePathFromSpecEnv(kb.Spec.PodTemplate.Spec)
	if err != nil {
		return "", err
	}

	if kbBasePath != "" {
		return kbBasePath, nil
	}

	cfg, err := getKibanaBasePathFromConfig(kb.Spec.Config)
	if err != nil {
		return "", err
	}

	return cfg.GetBasePath(), nil
}

func getKibanaBasePathFromConfig(cfg *v1.Config) (basePathConfig, error) {
	if cfg == nil {
		return basePathConfig{}, nil
	}

	kbucfgConfig, err := ucfg.NewFrom(cfg.Data, settings.Options...)
	if err != nil {
		return basePathConfig{}, err
	}

	kbCfg := basePathConfig{}
	if err := kbucfgConfig.Unpack(&kbCfg); err != nil {
		return basePathConfig{}, err
	}

	return kbCfg, nil
}

// GetKibanaContainer returns the Kibana container from the given podSpec.
func GetKibanaContainer(podSpec corev1.PodSpec) *corev1.Container {
	_, c := getKibanaContainerWithIndex(podSpec)
	return c
}

func getKibanaContainerWithIndex(podSpec corev1.PodSpec) (int, *corev1.Container) {
	return pod.ContainerAndIndexByName(podSpec.Containers, KibanaContainerName)
}

func getKibanaBasePathFromSpecEnv(podSpec corev1.PodSpec) (string, error) {
	cfg, err := getKibanaBasePathFromContainerEnv(GetKibanaContainer(podSpec))
	if err != nil {
		return "", err
	}

	return cfg.GetBasePath(), nil
}

func getKibanaBasePathFromContainerEnv(container *corev1.Container) (basePathConfig, error) {
	if container == nil {
		return basePathConfig{}, nil
	}

	cfg := basePathConfig{}

	for _, envVar := range container.Env {
		switch envVar.Name {
		case serverBasePathEnvName:
			cfg.Server.BasePath = &envVar.Value
		case serverRewriteBasePathEnvName:
			rewriteBasePathBool, err := strconv.ParseBool(envVar.Value)
			if err != nil {
				return basePathConfig{}, fmt.Errorf("failed to parse SERVER_REWRITEBASEPATH value %s: %w", envVar.Value, err)
			}
			cfg.Server.RewriteBasePath = &rewriteBasePathBool
		}
	}

	return cfg, nil
}
