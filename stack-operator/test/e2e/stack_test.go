package e2e

import (
	"bufio"
	"os"
	"testing"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis/deployments/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/test/e2e/helpers"
	"github.com/elastic/stack-operators/stack-operator/test/e2e/stack"
	"k8s.io/apimachinery/pkg/util/yaml"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp" // auth on gke
)

// Re-use the sample stack for e2e tests.
// This is a way to make sure both the sample and the e2e tests are always up-to-date.
// Path is relative to the e2e directory.
const sampleStackFile = "../fixture/deployments_v1alpha1_stack.yaml"

// TestStackSample runs a complete test suite using the sample stack
func TestStackSample(t *testing.T) {

	// build stack from yaml sample
	var sampleStack v1alpha1.Stack
	yamlFile, err := os.Open(sampleStackFile)
	helpers.ExitOnErr(err)
	err = yaml.NewYAMLToJSONDecoder(bufio.NewReader(yamlFile)).Decode(&sampleStack)
	helpers.ExitOnErr(err)

	// set namespace
	sampleStack.ObjectMeta.Namespace = helpers.DefaultNamespace

	// create k8s client
	k, err := helpers.NewK8sClient()
	helpers.ExitOnErr(err)

	helpers.TestSuite{}.
		// preliminary tests
		WithTestSteps(stack.InitTestSteps(sampleStack, k)...).
		// stack creation
		WithTestSteps(stack.CreationTestSteps(sampleStack, k)...).
		// stack mutation
		WithTestSteps(stack.MutationTestSteps(sampleStack, k)...).
		// stack deletion
		WithTestSteps(stack.DeletionTestSteps(sampleStack, k)...).
		// run!
		RunSequential(t)
}
