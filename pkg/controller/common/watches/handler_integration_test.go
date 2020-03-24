// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// +build integration

package watches_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/test"
)

func TestMain(m *testing.M) {
	test.RunWithK8s(m)
}

// TestDynamicEnqueueRequest tests the integration between a DynamicEnqueueRequest watch and
// a manager + controller, with a test k8s environment.
// The test just checks that everything fits together and reconciliations are correctly triggered
// from the EventHandler. More detailed behaviour is tested in `handler_test.go`.
func TestDynamicEnqueueRequest(t *testing.T) {
	eventHandler := watches.NewDynamicEnqueueRequest()
	// create a controller that watches secrets and enqueues requests into a chan
	requests := make(chan reconcile.Request)
	addToManager := func(mgr manager.Manager, params operator.Parameters) error {
		reconcileFunc := reconcile.Func(func(req reconcile.Request) (reconcile.Result, error) {
			requests <- req
			return reconcile.Result{}, nil
		})
		ctrl, err := controller.New("test-reconciler", mgr, controller.Options{Reconciler: reconcileFunc})
		require.NoError(t, err)
		require.NoError(t, ctrl.Watch(&source.Kind{Type: &corev1.Secret{}}, eventHandler))
		return nil
	}

	c, stop := test.StartManager(t, addToManager, operator.Parameters{})
	defer stop()

	// Fixtures
	watched := types.NamespacedName{
		Namespace: "default",
		Name:      "watched1",
	}
	testObj := &corev1.Secret{
		ObjectMeta: k8s.ToObjectMeta(watched),
	}
	watching := types.NamespacedName{
		Namespace: "default",
		Name:      "watcher",
	}

	// Create the object before registering any watches
	assert.NoError(t, c.Create(testObj))

	// Add a named watch for the first object
	assert.NoError(t, eventHandler.AddHandler(watches.NamedWatch{
		Watched: []types.NamespacedName{watched},
		Watcher: watching,
		Name:    "test-watch-1",
	}))

	// Update the first object and expect a reconcile request
	testLabels := map[string]string{"test": "label"}
	testObj.Labels = testLabels
	require.NoError(t, c.Update(testObj))
	require.Equal(t, watching, (<-requests).NamespacedName)
}
