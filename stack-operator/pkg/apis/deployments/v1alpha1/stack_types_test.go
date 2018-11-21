// +build integration

package v1alpha1

import (
	"fmt"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestStorageStack(t *testing.T) {
	key := types.NamespacedName{
		Name:      "foo",
		Namespace: "default",
	}
	created := &Stack{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		}}

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

func TestElasticsearchHealth_Less(t *testing.T) {

	tests := []struct {
		inputs []ElasticsearchHealth
		sorted bool
	}{
		{
			inputs: []ElasticsearchHealth{
				ElasticsearchRedHealth,
				ElasticsearchYellowHealth,
			},
			sorted: true,
		},
		{
			inputs: []ElasticsearchHealth{
				ElasticsearchRedHealth,
				ElasticsearchRedHealth,
			},
			sorted: true,
		},
		{
			inputs: []ElasticsearchHealth{
				ElasticsearchRedHealth,
				ElasticsearchGreenHealth,
			},
			sorted: true,
		},
		{
			inputs: []ElasticsearchHealth{
				ElasticsearchRedHealth,
				ElasticsearchYellowHealth,
				ElasticsearchGreenHealth,
			},
			sorted: true,
		},
		{
			inputs: []ElasticsearchHealth{
				ElasticsearchYellowHealth,
				ElasticsearchGreenHealth,
			},
			sorted: true,
		},
		{
			inputs: []ElasticsearchHealth{
				ElasticsearchGreenHealth,
				ElasticsearchYellowHealth,
			},
			sorted: false,
		},
	}

	for _, tt := range tests {
		assert.Equal(t, sort.SliceIsSorted(tt.inputs, func(i, j int) bool {
			return tt.inputs[i].Less(tt.inputs[j])
		}), tt.sorted, fmt.Sprintf("%v", tt.inputs))
	}
}
