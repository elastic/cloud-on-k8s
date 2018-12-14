package v1alpha1

import (
	"testing"

	"github.com/elastic/stack-operators/stack-operator/pkg/utils/k8s"
	"github.com/onsi/gomega"
	"golang.org/x/net/context"
)

func TestStorageKibana(t *testing.T) {
	key := k8s.NamespacedName(k8s.DefaultNamespace, "foo")
	created := &Kibana{
		ObjectMeta: k8s.ObjectMeta(k8s.DefaultNamespace, "foo"),
	}
	g := gomega.NewGomegaWithT(t)

	// Test Create
	fetched := &Kibana{}
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
