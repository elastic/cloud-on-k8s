// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Condition represents Elasticsearch resource's condition.
// **This API is in technical preview and may be changed or removed in a future release.**
type Condition struct {
	Type   ConditionType          `json:"type"`
	Status corev1.ConditionStatus `json:"status"`
	// +optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
	// +optional
	Message string `json:"message,omitempty"`
}

type Conditions []Condition

// ConditionType defines the condition of an Elasticsearch resource.
type ConditionType string

func (c Conditions) Index(conditionType ConditionType) int {
	for i, condition := range c {
		if condition.Type == conditionType {
			return i
		}
	}
	return -1
}

func (c Conditions) MergeWith(nextConditions ...Condition) Conditions {
	cp := c.DeepCopy()
	for i := range nextConditions {
		nextCondition := nextConditions[i]
		if index := cp.Index(nextCondition.Type); index >= 0 {
			currentCondition := c[index]
			if currentCondition.Status != nextCondition.Status ||
				currentCondition.Message != nextCondition.Message {
				// Update condition
				cp[index] = nextCondition
			}
		} else {
			cp = append(cp, nextCondition)
		}
	}
	return cp
}
