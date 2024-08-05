// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package runner

import (
	"fmt"
)

const ProjectTag = "eck-ci"

var (
	// elasticTags are tags to apply the Elastic Cloud resources tagging policy
	elasticTags = map[string]string{
		"division": "engineering",
		"org":      "controlplane",
		"team":     "cloud-k8s-operator",
		"project":  ProjectTag,
	}
)

// toList transforms a map into a slice of string where each element corresponds
// to an entry in the map represented in the form 'key=value'.
func toList(m map[string]string) []string {
	l := make([]string, 0, len(m))
	for k, v := range m {
		l = append(l, fmt.Sprintf("%s=%s", k, v))
	}
	return l
}
