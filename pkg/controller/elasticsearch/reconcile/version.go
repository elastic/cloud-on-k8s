// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package reconcile

import esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"

type OrchestrationVersion uint64

const (
	NoTransients OrchestrationVersion = 1 << iota
	NoDisabledAllocation
)

func set(status esv1.ElasticsearchStatus, flag OrchestrationVersion) esv1.ElasticsearchStatus {
	status.OrchestrationVersion = int(flag | OrchestrationVersion(status.OrchestrationVersion))
	return status
}
func HasOrchestrationFlag(es esv1.Elasticsearch, flag OrchestrationVersion) bool {
	return OrchestrationVersion(es.Status.OrchestrationVersion)&flag != 0
}
