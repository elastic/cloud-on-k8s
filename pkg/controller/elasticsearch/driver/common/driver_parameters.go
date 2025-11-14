// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package common

import (
	"k8s.io/client-go/tools/record"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/expectations"
	commonlicense "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/watches"
	esclient "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/observer"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

func NewDefaultDriverParameters(
	operatorParams operator.Parameters,
	es esv1.Elasticsearch,
	reconcileState *reconcile.State,
	client k8s.Client,
	recorder record.EventRecorder,
	version version.Version,
	expectations *expectations.Expectations,
	observers *observer.Manager,
	dynamicWatches watches.DynamicWatches,
	supportedVersions version.MinMaxVersion,
	licenseChecker commonlicense.Checker,
) *DefaultDriverParameters {
	return &DefaultDriverParameters{
		OperatorParameters: operatorParams,
		ES:                 es,
		SupportedVersions:  supportedVersions,
		Version:            version,
		Client:             client,
		recorder:           recorder,
		LicenseChecker:     licenseChecker,
		ReconcileState:     reconcileState,
		Observers:          observers,
		dynamicWatches:     dynamicWatches,
		Expectations:       expectations,
	}
}

// DefaultDriverParameters contain parameters for this driver.
// Most of them are persisted across driver creations.
type DefaultDriverParameters struct {
	// OperatorParameters contain global parameters about the operator.
	OperatorParameters operator.Parameters

	// ES is the Elasticsearch resource to reconcile
	ES esv1.Elasticsearch
	// SupportedVersions verifies whether we can support upgrading from the current pods.
	SupportedVersions version.MinMaxVersion

	// Version is the version of Elasticsearch we want to reconcile towards.
	Version version.Version
	// Client is used to access the Kubernetes API.
	Client k8s.Client

	recorder record.EventRecorder

	// LicenseChecker is used for some features to check if an appropriate license is setup
	LicenseChecker commonlicense.Checker

	// State holds the accumulated state during the reconcile loop
	ReconcileState *reconcile.State
	// Observers that observe es clusters state.
	Observers *observer.Manager
	// dynamicWatches are handles to currently registered dynamic watches.
	dynamicWatches watches.DynamicWatches
	// Expectations control some expectations set on resources in the cache, in order to
	// avoid doing certain operations if the cache hasn't seen an up-to-date resource yet.
	Expectations *expectations.Expectations
}

func (d *DefaultDriverParameters) K8sClient() k8s.Client {
	return d.Client
}

func (d *DefaultDriverParameters) DynamicWatches() watches.DynamicWatches {
	return d.dynamicWatches
}

func (d *DefaultDriverParameters) Recorder() record.EventRecorder {
	return d.recorder
}

type DefaultDriverParametersResult struct {
	*reconciler.Results

	Meta metadata.Metadata

	ResourcesState *reconcile.ResourcesState

	EsClient esclient.Client

	EsReachable bool
}

func (d *DefaultDriverParametersResult) Close() {
	if d == nil {
		return
	}
	if d.EsClient != nil {
		d.EsClient.Close()
	}
}

func (d *DefaultDriverParametersResult) WithError(err error) *DefaultDriverParametersResult {
	d.Results.WithError(err)
	return d
}

func (d *DefaultDriverParametersResult) WithReconciliationState(res reconciler.ReconciliationState) *DefaultDriverParametersResult {
	d.Results.WithReconciliationState(res)
	return d
}
