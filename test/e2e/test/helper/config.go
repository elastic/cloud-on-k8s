// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build mixed || e2e

package helper

import (
	"context"
	"fmt"

	"github.com/ghodss/yaml"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test"
)

func RemoveFromOperatorConfig(k k8s.Client, key string) error {
	return UpdateOperatorConfig(k, func(cfg map[string]any) {
		delete(cfg, key)
	})
}

func AddToOperatorConfig(k k8s.Client, key, value string) error {
	return UpdateOperatorConfig(k, func(cfg map[string]any) {
		cfg[key] = value
	})
}

func UpdateOperatorConfig(k k8s.Client, f func(map[string]any)) error {
	cm, config, err := getOperatorConfig(k)
	if err != nil {
		return err
	}
	f(config)
	bytes, err := yaml.Marshal(config)
	if err != nil {
		return err
	}
	cm.Data["eck.yaml"] = string(bytes)
	return k.Update(context.Background(), cm)
}

func SetOperatorConfig(ctx context.Context, k k8s.Client, cnf map[string]any) error {
	cm, _, err := getOperatorConfig(k)
	if err != nil {
		return err
	}

	bytes, err := yaml.Marshal(cnf)
	if err != nil {
		return err
	}
	cm.Data["eck.yaml"] = string(bytes)
	return k.Update(ctx, cm)
}

// GetOperatorConfig returns the operator configuration (the eck.yaml entry of the operator
// ConfigMap) as a map.
func GetOperatorConfig(k k8s.Client) (map[string]any, error) {
	_, config, err := getOperatorConfig(k)
	return config, err
}

func GetOperatorConfigValue(k k8s.Client, key string) (any, error) {
	config, err := GetOperatorConfig(k)
	if err != nil {
		return nil, err
	}
	return config[key], nil
}

// getOperatorConfig fetches the operator ConfigMap and unmarshals its eck.yaml entry.
func getOperatorConfig(k k8s.Client) (*corev1.ConfigMap, map[string]any, error) {
	var cm corev1.ConfigMap
	if err := k.Get(
		context.Background(),
		types.NamespacedName{Name: fmt.Sprintf("%s-operator", test.Ctx().TestRun), Namespace: test.Ctx().Operator.Namespace},
		&cm,
	); err != nil {
		return nil, nil, err
	}
	config := map[string]any{}
	if err := yaml.Unmarshal([]byte(cm.Data["eck.yaml"]), &config); err != nil {
		return nil, nil, err
	}
	return &cm, config, nil
}

func OperatorRestartCount(k *test.K8sClient) (int32, error) {
	pods, err := k.GetPods(test.OperatorPodListOptions(test.Ctx().Operator.Namespace)...)
	if err != nil {
		return 0, err
	}
	for _, p := range pods {
		for _, c := range p.Status.ContainerStatuses {
			if c.Name == "manager" {
				return c.RestartCount, nil
			}
		}
	}
	return 0, fmt.Errorf("could not find operator container")
}
