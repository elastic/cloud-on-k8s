package test

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// CheckReconcileCalled waits up to Timeout to receive the expected request on requests.
func CheckReconcileCalled(t *testing.T, requests chan reconcile.Request, expected reconcile.Request) {
	select {
	case req := <-requests:
		assert.Equal(t, req, expected)
	case <-time.After(Timeout):
		assert.Fail(t, fmt.Sprintf("No request received after %s", Timeout))
	}
}

// DeleteIfExists manually deletes the given object.
func DeleteIfExists(t *testing.T, c client.Client, obj runtime.Object) {
	if err := c.Delete(context.TODO(), obj); err != nil && !apierrors.IsNotFound(err) {
		// If the resource is already deleted, we don't care, but any other error is important
		assert.NoError(t, err)
	}
}

// CheckResourceDeletionTriggersReconcile deletes the given resource and tests for recreation.
func CheckResourceDeletionTriggersReconcile(
	t *testing.T,
	c client.Client,
	requests chan reconcile.Request,
	objKey types.NamespacedName,
	obj runtime.Object,
	expected reconcile.Request,
) {
	assert.NoError(t, c.Delete(context.TODO(), obj))
	CheckReconcileCalled(t, requests, expected)
	RetryUntilSuccess(t, func() error { return c.Get(context.TODO(), objKey, obj) })
}
