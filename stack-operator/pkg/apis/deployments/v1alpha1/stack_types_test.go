// +build integration

package v1alpha1

import (
	"testing"

	"github.com/elastic/stack-operators/stack-operator/pkg/utils/k8s"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
)

func TestStorageStack(t *testing.T) {
	key := k8s.NamespacedName(k8s.DefaultNamespace, "foo")
	created := &Stack{ObjectMeta: k8s.ObjectMeta(k8s.DefaultNamespace, "foo")}

	// Test Create
	fetched := &Stack{}

	assert.NoError(t, c.Create(context.TODO(), created))

	assert.NoError(t, c.Get(context.TODO(), key, fetched))
	assert.Equal(t, created, fetched)

	// Test Updating the Labels
	updated := fetched.DeepCopy()
	updated.Labels = map[string]string{"hello": "world"}
	assert.NoError(t, c.Update(context.TODO(), updated))

	assert.NoError(t, c.Get(context.TODO(), key, fetched))
	assert.Equal(t, fetched, updated)

	// Test Delete
	assert.NoError(t, c.Delete(context.TODO(), fetched))
	assert.Error(t, c.Get(context.TODO(), key, fetched))
}
