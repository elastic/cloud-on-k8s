package test

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"testing"
	"time"
)
// CheckReconcileCalled waits for Timeout to receive the expected requests on requests.
func CheckReconcileCalled(t *testing.T, requests chan reconcile.Request, expected interface{}) {
	select {
	case req := <-requests:
		assert.Equal(t, req, expected)
	case <-time.After(Timeout):
		assert.Fail(t, fmt.Sprintf("No request received after %s", Timeout))
	}
}

// Clean manually deletes the given object.
func Clean(t *testing.T, c client.Client, obj runtime.Object) {
	err := c.Delete(context.TODO(), obj)
	// If the resource is already deleted, we don't care, but any other error is important
	if !apierrors.IsNotFound(err) {
		assert.NoError(t, err)
	}
}

