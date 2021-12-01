// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package hints

import "encoding/json"

const OrchestrationsHintsAnnotation string = "eck.k8s.elastic.co/orchestration-hints"

// OrchestrationsHints represent hints to the reconciler about use or non-use of certain Elasticsearch feature for
// orchestration purposes.
type OrchestrationsHints struct {
	NoTransientSettings bool `json:"no_transient_settings"`
}

// Merge merges the hints in other into the receiver.
func (oh OrchestrationsHints) Merge(other OrchestrationsHints) OrchestrationsHints {
	return OrchestrationsHints{
		NoTransientSettings: oh.NoTransientSettings || other.NoTransientSettings,
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
