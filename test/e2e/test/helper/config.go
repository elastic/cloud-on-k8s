// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build mixed || e2e

package helper

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

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

const (
	// effectiveConfigLogMessage is the message of the log line, emitted by the operator at startup with log verbosity
	// 1 or higher, which contains the configuration the operator process runs with.
	effectiveConfigLogMessage = "Effective configuration"
	// effectiveConfigLogLimitBytes bounds the amount of logs read from the beginning of the operator logs to look for
	// the effective configuration log line, which is one of the first lines logged at startup.
	effectiveConfigLogLimitBytes = 64 * 1024
)

// parseEffectiveConfig extracts the configuration values from the effective configuration log line.
func parseEffectiveConfig(logs []byte) (map[string]any, error) {
	scanner := bufio.NewScanner(bytes.NewReader(logs))
	scanner.Buffer(make([]byte, 0, effectiveConfigLogLimitBytes), effectiveConfigLogLimitBytes)
	for scanner.Scan() {
		var line struct {
			Message string         `json:"message"`
			Values  map[string]any `json:"values"`
		}
		// skip non-JSON lines, and the last line which may have been truncated
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			continue
		}
		if line.Message == effectiveConfigLogMessage {
			return line.Values, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("while looking for the %q log line: %w", effectiveConfigLogMessage, err)
	}
	return nil, fmt.Errorf("%q log line not found, operator log verbosity must be 1 or higher", effectiveConfigLogMessage)
}

// CheckOperatorPodsLoadedConfig returns an error unless every operator Pod is ready and its running manager container
// was started with a configuration matching expected for each of the given keys. The configuration a process runs
// with is read from the effective configuration it logs at startup, which requires an operator log verbosity of 1 or
// higher. Checking every Pod, and not only one, matters when the operator runs multiple replicas: a standby replica
// still running with a previous configuration could otherwise acquire the leader lease and reconcile with it.
func CheckOperatorPodsLoadedConfig(ctx context.Context, k *test.K8sClient, expected map[string]any, keys ...string) error {
	pods, err := k.GetPods(test.OperatorPodListOptions(test.Ctx().Operator.Namespace)...)
	if err != nil {
		return err
	}
	if len(pods) == 0 {
		return fmt.Errorf("no operator Pod found")
	}
	for _, p := range pods {
		if !k8s.IsPodReady(p) {
			return fmt.Errorf("operator Pod %s is not ready", p.Name)
		}
		logs, err := k.GetPodLogs(ctx, k8s.ExtractNamespacedName(&p), "manager", effectiveConfigLogLimitBytes)
		if err != nil {
			return err
		}
		loaded, err := parseEffectiveConfig(logs)
		if err != nil {
			return fmt.Errorf("operator Pod %s: %w", p.Name, err)
		}
		for _, key := range keys {
			if want, got := normalizeConfigValue(expected[key]), normalizeConfigValue(loaded[key]); !reflect.DeepEqual(want, got) {
				return fmt.Errorf("operator Pod %s runs with %s=%v, expected %v", p.Name, key, got, want)
			}
		}
	}
	return nil
}

// normalizeConfigValue converts a configuration value into a form that can be compared regardless of its origin
// (operator ConfigMap or effective configuration log line): map keys are lowercased as the operator configuration
// keys are case-insensitive, and empty values are treated as unset.
func normalizeConfigValue(v any) any {
	switch val := v.(type) {
	case map[string]any:
		if len(val) == 0 {
			return nil
		}
		normalized := make(map[string]any, len(val))
		for key, item := range val {
			normalized[strings.ToLower(key)] = normalizeConfigValue(item)
		}
		return normalized
	case []any:
		if len(val) == 0 {
			return nil
		}
		normalized := make([]any, len(val))
		for i, item := range val {
			normalized[i] = normalizeConfigValue(item)
		}
		return normalized
	case []string:
		if len(val) == 0 {
			return nil
		}
		normalized := make([]any, len(val))
		for i, item := range val {
			normalized[i] = item
		}
		return normalized
	case string:
		if val == "" {
			return nil
		}
		return val
	default:
		return val
	}
}
