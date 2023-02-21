// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package runner

import (
	"fmt"
	"strings"
)

var (
	// elasticTags are tags to apply the Elastic Cloud resources tagging policy
	elasticTags = map[string]string{
		"division": "engineering",
		"org":      "controlplane",
		"team":     "cloud-k8s-operator",
		"project":  "eck-ci",
	}
)

func toKVList(m map[string]string) []string {
	l := []string{}
	for k, v := range m {
		l = append(l, fmt.Sprintf("%s=%s", k, v))
	}
	return l
}

// https://eksctl.io/usage/schema/#metadata-tags
func eksElasticTags() map[string]string {
	return elasticTags
}

// https://learn.microsoft.com/en-us/cli/azure/aks?view=azure-cli-latest#az-aks-create
// https://learn.microsoft.com/en-us/cli/azure/group?view=azure-cli-latest#az-group-create
func azureElasticTags() string {
	return strings.Join(toKVList(elasticTags), " ")
}

// https://cloud.google.com/kubernetes-engine/docs/how-to/tags
// https://cloud.google.com/kubernetes-engine/docs/how-to/creating-managing-labels
func gkeElasticLabels() string {
	return strings.Join(toKVList(elasticTags), ",")
}
