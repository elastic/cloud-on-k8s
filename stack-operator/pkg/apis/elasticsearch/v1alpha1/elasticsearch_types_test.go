package v1alpha1

import (
	"fmt"
	"sort"
	"testing"

	"github.com/elastic/stack-operators/stack-operator/pkg/utils/k8s"
	"github.com/stretchr/testify/assert"

	"github.com/onsi/gomega"
	"golang.org/x/net/context"
)

func TestStorageElasticsearch(t *testing.T) {
	key := k8s.NamespacedName(k8s.DefaultNamespace, "foo")
	created := &ElasticsearchCluster{
		ObjectMeta: k8s.ObjectMeta(k8s.DefaultNamespace, "foo"),
		Spec:       ElasticsearchSpec{},
	}
	g := gomega.NewGomegaWithT(t)

	// Test Create
	fetched := &ElasticsearchCluster{}
	g.Expect(c.Create(context.TODO(), created)).NotTo(gomega.HaveOccurred())

	g.Expect(c.Get(context.TODO(), key, fetched)).NotTo(gomega.HaveOccurred())
	g.Expect(fetched).To(gomega.Equal(created))

	// Test Updating the Labels
	updated := fetched.DeepCopy()
	updated.Labels = map[string]string{"hello": "world"}
	g.Expect(c.Update(context.TODO(), updated)).NotTo(gomega.HaveOccurred())

	g.Expect(c.Get(context.TODO(), key, fetched)).NotTo(gomega.HaveOccurred())
	g.Expect(fetched).To(gomega.Equal(updated))

	// Test Delete
	g.Expect(c.Delete(context.TODO(), fetched)).NotTo(gomega.HaveOccurred())
	g.Expect(c.Get(context.TODO(), key, fetched)).To(gomega.HaveOccurred())
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
