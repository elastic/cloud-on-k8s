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
	var cm corev1.ConfigMap
	if err := k.Get(context.Background(),
		types.NamespacedName{Name: fmt.Sprintf("%s-operator", test.Ctx().TestRun), Namespace: test.Ctx().Operator.Namespace},
		&cm,
	); err != nil {
		return err
	}
	raw := cm.Data["eck.yaml"]
	config := map[string]any{}
	if err := yaml.Unmarshal([]byte(raw), &config); err != nil {
		return err
	}
	f(config)
	bytes, err := yaml.Marshal(config)
	if err != nil {
		return err
	}
	cm.Data["eck.yaml"] = string(bytes)
	return k.Update(context.Background(), &cm)
}

func GetOperatorConfigValue(k k8s.Client, key string) (any, error) {
	var result any
	err := UpdateOperatorConfig(k, func(cfg map[string]any) {
		result = cfg[key]
	})
	return result, err
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
