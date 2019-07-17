// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"fmt"
	"time"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/observer"
	esreconcile "github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/user"
	esversion "github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/version"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var (
	log = logf.Log.WithName("driver")

	defaultRequeue = reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}
)

// Driver is something that can reconcile an Elasticsearch resource
type Driver interface {
	Reconcile(
		es v1alpha1.Elasticsearch,
		reconcileState *esreconcile.State,
	) *reconciler.Results
}

// Options are used to create a driver. See NewDriver
type Options struct {
	operator.Parameters
	// Version is the version of Elasticsearch we want to reconcile towards
	Version version.Version
	// Client is used to access the Kubernetes API
	Client   k8s.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	// Observers that observe es clusters state
	Observers *observer.Manager
	// DynamicWatches are handles to currently registered dynamic watches.
	DynamicWatches watches.DynamicWatches
	// Expectations control some expectations set on resources in the cache, in order to
	// avoid doing certain operations if the cache hasn't seen an up-to-date resource yet.
	Expectations *Expectations
}

// NewDriver returns a Driver that can operate the provided version
func NewDriver(opts Options) (Driver, error) {
	supported := esversion.SupportedVersions(opts.Version)
	if supported == nil {
		return nil, fmt.Errorf("unsupported version: %s", opts.Version)
	}
	driver := &defaultDriver{
		Options: opts,

		expectations:           NewGenerationExpectations(),
		observedStateResolver:  opts.Observers.ObservedStateResolver,
		resourcesStateResolver: esreconcile.NewResourcesStateFromAPI,
		usersReconciler:        user.ReconcileUsers,
		supportedVersions:      *supported,
	}

	return driver, nil
}
