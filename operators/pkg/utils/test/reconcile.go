package test

import (
	"fmt"
	"testing"
	"time"

	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	"github.com/stretchr/testify/assert"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// CheckReconcileCalledIn waits up to Timeout to receive the expected request on requests.
func CheckReconcileCalledIn(t *testing.T, requests chan reconcile.Request, expected reconcile.Request, min, max int) {
	var seen int
	for seen < max {
		select {
		case req := <-requests:
			seen++
			assert.Equal(t, req, expected)
		case <-time.After(Timeout / time.Duration(max)):
			if seen < min {
				assert.Fail(t, fmt.Sprintf("No request received after %s", Timeout))
			}
			return
		}
	}
}

// CheckReconcileCalled waits up to Timeout to receive the expected request on requests.
func CheckReconcileCalled(t *testing.T, requests chan reconcile.Request, expected reconcile.Request) {
	select {
	case req := <-requests:
		assert.Equal(t, req, expected)
	case <-time.After(Timeout):
		assert.Fail(t, fmt.Sprintf("No request received after %s", Timeout))
	}
}

// CheckReconcileNotCalled ensures that no reconcile requests are currently pending
func CheckReconcileNotCalledWithin(t *testing.T, requests chan reconcile.Request, duration time.Duration) {
	select {
	case req := <-requests:
		assert.Fail(t, fmt.Sprintf("No request expected but got %v", req))
	case <-time.After(duration):
		//no request received, OK moving on
	}
}

// DeleteIfExists manually deletes the given object.
func DeleteIfExists(t *testing.T, c k8s.Client, obj runtime.Object) {
	if err := c.Delete(obj); err != nil && !apierrors.IsNotFound(err) {
		// If the resource is already deleted, we don't care, but any other error is important
		assert.NoError(t, err)
	}
}

// CheckResourceDeletionTriggersReconcile deletes the given resource and tests for recreation.
func CheckResourceDeletionTriggersReconcile(
	t *testing.T,
	c k8s.Client,
	requests chan reconcile.Request,
	objKey types.NamespacedName,
	obj runtime.Object,
	expected reconcile.Request,
) {
	assert.NoError(t, c.Delete(obj))
	CheckReconcileCalled(t, requests, expected)
	RetryUntilSuccess(t, func() error { return c.Get(objKey, obj) })
}
