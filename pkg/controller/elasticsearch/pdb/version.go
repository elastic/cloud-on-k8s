// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package pdb

import (
	"reflect"
	"sync"

	policyv1 "k8s.io/api/policy/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"

	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

var (
	pdbVersionMutex sync.RWMutex
	pdbV1Available  *bool
)

// convert converts v1 version of the PodDisruptionBudget resource to v1beta1
func convert(toConvert *policyv1.PodDisruptionBudget) *policyv1beta1.PodDisruptionBudget {
	v1beta1 := &policyv1beta1.PodDisruptionBudget{}
	v1beta1.ObjectMeta = toConvert.ObjectMeta
	v1beta1.Spec.MinAvailable = toConvert.Spec.MinAvailable
	v1beta1.Spec.Selector = toConvert.Spec.Selector
	v1beta1.Spec.MaxUnavailable = toConvert.Spec.MaxUnavailable
	return v1beta1
}

func isPDBV1Available(k8sClient k8s.Client) (bool, error) {
	isPDBV1Available := getPDBV1Available()
	if isPDBV1Available != nil {
		return *isPDBV1Available, nil
	}
	return initPDBV1Available(k8sClient)
}

func getPDBV1Available() *bool {
	pdbVersionMutex.RLock()
	defer pdbVersionMutex.RUnlock()
	return pdbV1Available
}

func initPDBV1Available(k8sClient k8s.Client) (bool, error) {
	pdbVersionMutex.Lock()
	defer pdbVersionMutex.Unlock()
	if pdbV1Available != nil {
		return *pdbV1Available, nil
	}
	t := reflect.TypeOf(&policyv1.PodDisruptionBudget{})
	gk := schema.GroupKind{
		Group: policyv1.GroupName,
		Kind:  t.Elem().Name(),
	}
	preferredMapping, err := k8sClient.RESTMapper().RESTMapping(gk)
	if err != nil {
		return false, err
	}

	// Rely on v1 as soon as v1beta1 is not the preferred version anymore.
	pdbV1Available = ptr.To[bool](preferredMapping.Resource.Version != "v1beta1")
	return *pdbV1Available, nil
}
