// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package helper

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	apmv1 "github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1"
	beatv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/beat/v1beta1"
	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	entv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/enterprisesearch/v1beta1"
	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	beatcommon "github.com/elastic/cloud-on-k8s/pkg/controller/beat/common"
	"github.com/elastic/cloud-on-k8s/test/e2e/cmd/run"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/apmserver"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/beat"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/elasticsearch"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/enterprisesearch"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/kibana"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	meta2 "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/yaml"
)

type BuilderTransform func(test.Builder) test.Builder

// YAMLDecoder converts YAML bytes into test.Builder instances.
type YAMLDecoder struct {
	decoder runtime.Decoder
}

func NewYAMLDecoder() *YAMLDecoder {
	scheme := runtime.NewScheme()
	scheme.AddKnownTypes(esv1.GroupVersion, &esv1.Elasticsearch{}, &esv1.ElasticsearchList{})
	scheme.AddKnownTypes(kbv1.GroupVersion, &kbv1.Kibana{}, &kbv1.KibanaList{})
	scheme.AddKnownTypes(apmv1.GroupVersion, &apmv1.ApmServer{}, &apmv1.ApmServerList{})
	scheme.AddKnownTypes(beatv1beta1.GroupVersion, &beatv1beta1.Beat{}, &beatv1beta1.BeatList{})
	scheme.AddKnownTypes(entv1beta1.GroupVersion, &entv1beta1.EnterpriseSearch{}, &entv1beta1.EnterpriseSearchList{})

	scheme.AddKnownTypes(rbacv1.SchemeGroupVersion, &rbacv1.ClusterRoleBinding{}, &rbacv1.ClusterRoleBindingList{})
	scheme.AddKnownTypes(rbacv1.SchemeGroupVersion, &rbacv1.ClusterRole{}, &rbacv1.ClusterRoleList{})
	scheme.AddKnownTypes(corev1.SchemeGroupVersion, &corev1.ServiceAccount{}, &corev1.ServiceAccountList{})
	decoder := serializer.NewCodecFactory(scheme).UniversalDeserializer()

	return &YAMLDecoder{decoder: decoder}
}

func (yd *YAMLDecoder) ToBuilders(reader *bufio.Reader, transform BuilderTransform) ([]test.Builder, error) {
	var builders []test.Builder

	yamlReader := yaml.NewYAMLReader(reader)
	for {
		yamlBytes, err := yamlReader.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("failed to read YAML: %w", err)
		}
		obj, _, err := yd.decoder.Decode(yamlBytes, nil, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to decode YAML: %w", err)
		}

		var builder test.Builder

		switch decodedObj := obj.(type) {
		case *esv1.Elasticsearch:
			b := elasticsearch.NewBuilderWithoutSuffix(decodedObj.Name)
			b.Elasticsearch = *decodedObj
			builder = transform(b)
		case *kbv1.Kibana:
			b := kibana.NewBuilderWithoutSuffix(decodedObj.Name)
			b.Kibana = *decodedObj
			builder = transform(b)
		case *apmv1.ApmServer:
			b := apmserver.NewBuilderWithoutSuffix(decodedObj.Name)
			b.ApmServer = *decodedObj
			builder = transform(b)
		case *beatv1beta1.Beat:
			b := beat.NewBuilderFromBeat(decodedObj)
			builder = transform(b)
		case *entv1beta1.EnterpriseSearch:
			b := enterprisesearch.NewBuilderWithoutSuffix(decodedObj.Name)
			b.EnterpriseSearch = *decodedObj
			builder = transform(b)
		default:
			return builders, fmt.Errorf("unexpected object type: %t", decodedObj)
		}

		builders = append(builders, builder)
	}

	return builders, nil
}

func (yd *YAMLDecoder) ToObjects(reader *bufio.Reader) ([]runtime.Object, error) {
	var objects []runtime.Object

	yamlReader := yaml.NewYAMLReader(reader)
	for {
		yamlBytes, err := yamlReader.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("failed to read YAML: %w", err)
		}
		obj, _, err := yd.decoder.Decode(yamlBytes, nil, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to decode YAML: %w", err)
		}

		objects = append(objects, obj)
	}

	return objects, nil
}

// RunFile runs the builder workflow for all known resources in a yaml file, all other objects are created before and deleted
// after. Resources will be created in a given namespace and with a given suffix. Additional objects to be created and deleted
// can be passed as well as set of optional transformations to apply to all Builders.
func RunFile(
	t *testing.T,
	filePath, namespace, suffix string,
	additionalObjects []runtime.Object,
	transformations ...BuilderTransform) {
	builders, objects, err := extractFromFile(t, filePath, namespace, suffix, MkTestName(t, filePath), transformations...)
	if err != nil {
		panic(err)
	}

	objects = append(objects, additionalObjects...)

	creates, deletes := makeObjectSteps(t, objects)

	test.BeforeAfterSequence(creates, deletes, builders...).RunSequential(t)
}

func extractFromFile(
	t *testing.T,
	filePath, namespace, suffix, fullTestName string,
	transformations ...BuilderTransform,
) ([]test.Builder, []runtime.Object, error) {
	f, err := os.Open(filePath)
	require.NoError(t, err, "Failed to open file %s", filePath)
	defer f.Close()

	decoder := NewYAMLDecoder()
	objects, err := decoder.ToObjects(bufio.NewReader(f))
	if err != nil {
		return nil, nil, err
	}

	builders, objects := transformToE2E(namespace, fullTestName, suffix, transformations, objects)
	return builders, objects, nil
}

func makeObjectSteps(
	t *testing.T,
	objects []runtime.Object,
) (func(k *test.K8sClient) test.StepList, func(k *test.K8sClient) test.StepList) {
	return func(k *test.K8sClient) test.StepList {
			steps := test.StepList{}
			for i := range objects {
				ii := i
				meta, err := meta2.Accessor(objects[ii])
				require.NoError(t, err)
				steps = steps.WithStep(test.Step{
					Name: fmt.Sprintf("Create %s/%s", meta.GetNamespace(), meta.GetNamespace()),
					Test: func(t *testing.T) {
						err := k.Client.Create(objects[ii])
						if !k8serrors.IsAlreadyExists(err) {
							require.NoError(t, err)
						}
					},
				})
			}
			return steps
		}, func(k *test.K8sClient) test.StepList {
			steps := test.StepList{}
			for i := range objects {
				ii := i
				meta, err := meta2.Accessor(objects[ii])
				require.NoError(t, err)
				steps = steps.WithStep(test.Step{
					Name: fmt.Sprintf("Delete %s/%s", meta.GetNamespace(), meta.GetNamespace()),
					Test: func(t *testing.T) {
						err := k.Client.Delete(objects[ii])
						if !k8serrors.IsNotFound(err) {
							require.NoError(t, err)
						}
					},
				})
			}
			return steps
		}
}

func transformToE2E(namespace, fullTestName, suffix string, transformers []BuilderTransform, objects []runtime.Object) ([]test.Builder, []runtime.Object) {
	var builders []test.Builder
	var otherObjects []runtime.Object
	for _, object := range objects {
		var builder test.Builder
		switch decodedObj := object.(type) {
		case *esv1.Elasticsearch:
			b := elasticsearch.NewBuilderWithoutSuffix(decodedObj.Name)
			b.Elasticsearch = *decodedObj
			builder = b.WithNamespace(namespace).
				WithSuffix(suffix).
				WithRestrictedSecurityContext().
				WithLabel(run.TestNameLabel, fullTestName).
				WithPodLabel(run.TestNameLabel, fullTestName)
		case *kbv1.Kibana:
			b := kibana.NewBuilderWithoutSuffix(decodedObj.Name)
			b.Kibana = *decodedObj
			builder = b.WithNamespace(namespace).
				WithSuffix(suffix).
				WithElasticsearchRef(tweakServiceRef(b.Kibana.Spec.ElasticsearchRef, suffix)).
				WithRestrictedSecurityContext().
				WithLabel(run.TestNameLabel, fullTestName).
				WithPodLabel(run.TestNameLabel, fullTestName)
		case *apmv1.ApmServer:
			b := apmserver.NewBuilderWithoutSuffix(decodedObj.Name)
			b.ApmServer = *decodedObj
			builder = b.WithNamespace(namespace).
				WithSuffix(suffix).
				WithElasticsearchRef(tweakServiceRef(b.ApmServer.Spec.ElasticsearchRef, suffix)).
				WithKibanaRef(tweakServiceRef(b.ApmServer.Spec.KibanaRef, suffix)).
				WithConfig(map[string]interface{}{"apm-server.ilm.enabled": false}).
				WithRestrictedSecurityContext().
				WithLabel(run.TestNameLabel, fullTestName).
				WithPodLabel(run.TestNameLabel, fullTestName)
		case *beatv1beta1.Beat:
			b := beat.NewBuilderFromBeat(decodedObj)

			builder = b.WithNamespace(namespace).
				WithSuffix(suffix).
				WithElasticsearchRef(tweakServiceRef(b.Beat.Spec.ElasticsearchRef, suffix)).
				WithLabel(run.TestNameLabel, fullTestName).
				WithPodLabel(run.TestNameLabel, fullTestName).
				WithESValidations(beat.HasEventFromBeat(beatcommon.Type(b.Beat.Spec.Type))).
				WithKibanaRef(tweakServiceRef(b.Beat.Spec.KibanaRef, suffix))

			if b.PodTemplate.Spec.ServiceAccountName != "" {
				b = b.WithPodTemplateServiceAccount(b.PodTemplate.Spec.ServiceAccountName + "-" + suffix)
			}
		case *entv1beta1.EnterpriseSearch:
			b := enterprisesearch.NewBuilderWithoutSuffix(decodedObj.Name)
			b.EnterpriseSearch = *decodedObj
			builder = b.WithNamespace(namespace).
				WithSuffix(suffix).
				WithElasticsearchRef(tweakServiceRef(b.EnterpriseSearch.Spec.ElasticsearchRef, suffix)).
				WithRestrictedSecurityContext().
				WithLabel(run.TestNameLabel, fullTestName).
				WithPodLabel(run.TestNameLabel, fullTestName)
		case *corev1.ServiceAccount:
			decodedObj.Namespace = namespace
			decodedObj.Name = decodedObj.Name + "-" + suffix
		case *rbacv1.ClusterRoleBinding:
			decodedObj.Subjects[0].Namespace = namespace
			decodedObj.Subjects[0].Name = decodedObj.Subjects[0].Name + "-" + suffix
			decodedObj.RoleRef.Name = decodedObj.RoleRef.Name + "-" + suffix
			decodedObj.Name = decodedObj.Name + "-" + suffix
		case *rbacv1.ClusterRole:
			decodedObj.Name = decodedObj.Name + "-" + suffix
		}

		if builder != nil {
			// ECK driven resources can be further transformed
			for _, transformer := range transformers {
				builder = transformer(builder)
			}
			builders = append(builders, builder)
		} else {
			// built-in in resources are separated as they are treated differently
			otherObjects = append(otherObjects, object)
		}
	}

	return builders, otherObjects
}

func tweakServiceRef(ref commonv1.ObjectSelector, suffix string) commonv1.ObjectSelector {
	// All the objects defined in the YAML file will have a random test suffix added to prevent clashes with previous runs.
	// This necessitates changing the Elasticsearch reference to match the suffixed name.
	if ref.Name != "" {
		ref.Name = ref.Name + "-" + suffix
	}

	return ref
}

func MkTestName(t *testing.T, path string) string {
	t.Helper()

	baseName := filepath.Base(path)
	baseName = strings.TrimSuffix(baseName, ".yaml")
	parentDir := filepath.Base(filepath.Dir(path))
	testName := filepath.Join(parentDir, baseName)

	// testName will be used as label, so avoid using illegal chars
	return strings.ReplaceAll(testName, "/", "-")
}
