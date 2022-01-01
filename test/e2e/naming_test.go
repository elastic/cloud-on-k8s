// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

// +build mixed e2e

package e2e

import (
	"context"
	"fmt"
	"hash/fnv"
	"strconv"
	"strings"
	"testing"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/name"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/apmserver"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/elasticsearch"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/kibana"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/util/validation"
)

func TestNameValidation(t *testing.T) {
	t.Run("longestPossibleName", testLongestPossibleName)
	t.Run("rejectionOfLongName", testRejectionOfLongName)
}

func testLongestPossibleName(t *testing.T) {
	maxESNameLen := name.MaxResourceNameLength
	randSuffix := rand.String(4)

	esNamePrefix := strings.Join([]string{"es-naming", randSuffix}, "-")
	esName := strings.Join([]string{esNamePrefix, strings.Repeat("x", maxESNameLen-len(esNamePrefix)-1)}, "-")

	// StatefulSet name would look like <esName>-es-<nodeSpecName>
	// Pods created by the StatefulSet will have the ordinal appended to the name and a controller revision hash
	// label created by appending a revision hash to the pod name.
	revisionHash := fnv.New32a()
	_, _ = revisionHash.Write([]byte("some random data"))
	fullRevisionHash := rand.SafeEncodeString(strconv.FormatInt(int64(revisionHash.Sum32()), 10))

	maxNodeSpecNameLen := validation.LabelValueMaxLength - len(esName) - len("-es-") - len("-0") - len(fmt.Sprintf("-%s", fullRevisionHash))
	nodeSpecName := strings.Repeat("y", maxNodeSpecNameLen)
	esBuilder := elasticsearch.NewBuilderWithoutSuffix(esName).
		WithESMasterDataNodes(3, elasticsearch.DefaultResources).
		WithNamespace(test.Ctx().ManagedNamespace(0)).
		WithVersion(test.Ctx().ElasticStackVersion).
		WithNodeSet(esv1.NodeSet{
			Name:  nodeSpecName,
			Count: 1,
		}).
		WithRestrictedSecurityContext()

	kbNamePrefix := strings.Join([]string{esNamePrefix, "kb"}, "-")
	kbName := strings.Join([]string{kbNamePrefix, strings.Repeat("x", name.MaxResourceNameLength-len(kbNamePrefix)-1)}, "-")
	kbBuilder := kibana.NewBuilderWithoutSuffix(kbName).
		WithNamespace(test.Ctx().ManagedNamespace(0)).
		WithNodeCount(1).
		WithElasticsearchRef(esBuilder.Ref()).
		WithVersion(test.Ctx().ElasticStackVersion).
		WithRestrictedSecurityContext().
		WithAPMIntegration()

	apmNamePrefix := strings.Join([]string{esNamePrefix, "apm"}, "-")
	apmName := strings.Join([]string{apmNamePrefix, strings.Repeat("x", name.MaxResourceNameLength-len(apmNamePrefix)-1)}, "-")
	apmBuilder := apmserver.NewBuilderWithoutSuffix(apmName).
		WithNamespace(test.Ctx().ManagedNamespace(0)).
		WithNodeCount(1).
		WithElasticsearchRef(esBuilder.Ref()).
		WithVersion(test.Ctx().ElasticStackVersion).
		WithConfig(map[string]interface{}{
			"apm-server.ilm.enabled": false,
		}).
		WithRestrictedSecurityContext()

	test.Sequence(nil, test.EmptySteps, esBuilder, kbBuilder, apmBuilder).RunSequential(t)
}

func testRejectionOfLongName(t *testing.T) {
	k := test.NewK8sClientOrFatal()

	randSuffix := rand.String(4)
	esName := strings.Join([]string{"es-name-length", randSuffix, strings.Repeat("x", name.MaxResourceNameLength)}, "-")
	esBuilder := elasticsearch.NewBuilderWithoutSuffix(esName).
		WithESMasterDataNodes(1, elasticsearch.DefaultResources).
		WithNamespace(test.Ctx().ManagedNamespace(0)).
		WithVersion(test.Ctx().ElasticStackVersion).
		WithNodeSet(esv1.NodeSet{
			Name:  "default",
			Count: 1,
		}).
		WithRestrictedSecurityContext()

	objectCreated := false

	testSteps := test.StepList{
		test.Step{
			Name: "Creating an Elasticsearch object should fail validation",
			Test: func(t *testing.T) {
				for _, obj := range esBuilder.RuntimeObjects() {
					err := k.Client.Create(context.Background(), obj)
					if err != nil {
						// validating webhook is active and rejected the request
						require.Contains(t, err.Error(), `admission webhook "elastic-es-validation-v1.k8s.elastic.co" denied the request`)
						return
					}

					// if the validating webhook is not active, operator's own validation check should set the object phase to "Invalid"
					objectCreated = true
					test.Eventually(func() error {
						var createdES esv1.Elasticsearch
						if err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&esBuilder.Elasticsearch), &createdES); err != nil {
							return err
						}

						if createdES.Status.Phase != esv1.ElasticsearchResourceInvalid {
							return fmt.Errorf("expected phase=[%s], actual phase=[%s]", esv1.ElasticsearchResourceInvalid, createdES.Status.Phase)
						}
						return nil
					})(t)
				}
			},
		},
		test.Step{
			Name: "Deleting an invalid Elasticsearch object should succeed",
			Test: test.Eventually(func() error {
				// if the validating webhook rejected the request, we have nothing to delete
				if !objectCreated {
					return nil
				}

				for _, obj := range esBuilder.RuntimeObjects() {
					err := k.Client.Delete(context.Background(), obj)
					if err != nil && !apierrors.IsNotFound(err) {
						return err
					}
				}

				var createdES esv1.Elasticsearch
				err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&esBuilder.Elasticsearch), &createdES)
				if apierrors.IsNotFound(err) {
					return nil
				}

				return errors.Wrapf(err, "object should not exist")
			}),
		},
	}

	testSteps.RunSequential(t)
}
