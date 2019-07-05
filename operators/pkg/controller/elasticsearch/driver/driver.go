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
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/version/version6"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/version/version7"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"k8s.io/apimachinery/pkg/runtime"
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
	Client k8s.Client
	Scheme *runtime.Scheme

	// Observers that observe es clusters state
	Observers *observer.Manager
	// DynamicWatches are handles to currently registered dynamic watches.
	DynamicWatches watches.DynamicWatches
	// PodsExpectations control ongoing pod creations and deletions
	// that might not be in-sync yet with our k8s client cache
	PodsExpectations *reconciler.Expectations
}

// NewDriver returns a Driver that can operate the provided version
func NewDriver(opts Options) (Driver, error) {
	supported := SupportedVersions(opts.Version)
	if supported == nil {
		return nil, fmt.Errorf("unsupported version: %s", opts.Version)
	}
	driver := &defaultDriver{
		Options: opts,

		observedStateResolver:  opts.Observers.ObservedStateResolver,
		resourcesStateResolver: esreconcile.NewResourcesStateFromAPI,
		usersReconciler:        user.ReconcileUsers,
		supportedVersions:      *supported,
	}

	switch opts.Version.Major {
	case 7:
		//driver.expectedPodsAndResourcesResolver = version6.ExpectedPodSpecs

		driver.clusterInitialMasterNodesEnforcer = version7.ClusterInitialMasterNodesEnforcer

		// version 7 uses zen2 instead of zen
		driver.zen2SettingsUpdater = version7.UpdateZen2Settings
		// .. except we still have to manage minimum_master_nodes while doing a rolling upgrade from 6 -> 7
		// we approximate this by also handling zen 1, even in 7
		// TODO: only do this if there's 6.x masters in the cluster.
		driver.zen1SettingsUpdater = version6.UpdateZen1Discovery
	case 6:
		//driver.expectedPodsAndResourcesResolver = version6.ExpectedPodSpecs
		driver.zen1SettingsUpdater = version6.UpdateZen1Discovery
	default:
		return nil, fmt.Errorf("unsupported version: %s", opts.Version)
	}

	return driver, nil
}

func SupportedVersions(v version.Version) *esversion.LowestHighestSupportedVersions {
	var res *esversion.LowestHighestSupportedVersions
	switch v.Major {
	case 6:
		res = &esversion.LowestHighestSupportedVersions{
			// Min. version is 6.7.0 for now. Will be 6.8.0 soon.
			LowestSupportedVersion: version.MustParse("6.7.0"),
			// higher may be possible, but not proven yet, lower may also be a requirement...
			HighestSupportedVersion: version.MustParse("6.99.99"),
		}
	case 7:
		res = &esversion.LowestHighestSupportedVersions{
			// 6.7.0 is the lowest wire compatibility version for 7.x
			LowestSupportedVersion: version.MustParse("6.7.0"),
			// higher may be possible, but not proven yet, lower may also be a requirement...
			HighestSupportedVersion: version.MustParse("7.99.99"),
		}
	}
	return res
}
