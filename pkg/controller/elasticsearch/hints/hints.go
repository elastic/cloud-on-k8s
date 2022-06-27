// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package hints

import (
	"encoding/json"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/utils/optional"
)

const OrchestrationsHintsAnnotation string = "eck.k8s.elastic.co/orchestration-hints"

// OrchestrationsHints represent hints to the reconciler about use or non-use of certain Elasticsearch feature for
// orchestration purposes.
type OrchestrationsHints struct {
	NoTransientSettings bool `json:"no_transient_settings"`

	// ServiceAccounts is set to true if all the nodes in the Elasticsearch cluster can authenticate
	// (Elasticsearch) service accounts. More specifically, it means that all the Elasticsearch nodes are running a
	// version that supports service accounts, and all the nodes have been restarted at least once in order to create
	// a symbolic link from the Secret that contains the tokens to the Elasticsearch configuration directory.
	// Elasticsearch nodes cannot read the tokens created by the operator until that symbolic link exists, the association
	// controllers should then rely on regular users until the value is true.
	ServiceAccounts *optional.Bool `json:"service_accounts,omitempty"`
}

// Merge merges the hints in other into the receiver.
func (oh OrchestrationsHints) Merge(other OrchestrationsHints) OrchestrationsHints {
	return OrchestrationsHints{
		NoTransientSettings: oh.NoTransientSettings || other.NoTransientSettings,
		ServiceAccounts:     oh.ServiceAccounts.Or(other.ServiceAccounts),
	}
}

// AsAnnotation returns a representation of orchestration hints that can be used as an annotation on the
// Elasticsearch resource.
func (oh OrchestrationsHints) AsAnnotation() (map[string]string, error) {
	bytes, err := json.Marshal(oh)
	if err != nil {
		return nil, err
	}
	return map[string]string{
		OrchestrationsHintsAnnotation: string(bytes),
	}, nil
}

// NewFromAnnotations creates new orchestration hints from annotation metadata coming from the Elasticsearch resource.
func NewFromAnnotations(ann map[string]string) (OrchestrationsHints, error) {
	jsonStr, exists := ann[OrchestrationsHintsAnnotation]
	if !exists {
		return OrchestrationsHints{}, nil
	}
	var hs OrchestrationsHints
	if err := json.Unmarshal([]byte(jsonStr), &hs); err != nil {
		return OrchestrationsHints{}, err
	}
	return hs, nil
}

// NewFrom creates new orchestration hints from the Elasticsearch resource.
func NewFrom(es esv1.Elasticsearch) (OrchestrationsHints, error) {
	if es.Annotations == nil {
		return OrchestrationsHints{}, nil
	}
	return NewFromAnnotations(es.Annotations)
}
