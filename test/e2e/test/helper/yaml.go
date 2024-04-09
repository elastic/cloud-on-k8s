// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package helper

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	meta2 "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/agent/v1alpha1"
	apmv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/apm/v1"
	beatv1beta1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/beat/v1beta1"
	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	entv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/enterprisesearch/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/kibana/v1"
	logstashv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/logstash/v1alpha1"
	beatcommon "github.com/elastic/cloud-on-k8s/v2/pkg/controller/beat/common"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/cmd/run"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/agent"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/apmserver"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/beat"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/elasticsearch"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/enterprisesearch"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/kibana"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/logstash"
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
	scheme.AddKnownTypes(entv1.GroupVersion, &entv1.EnterpriseSearch{}, &entv1.EnterpriseSearchList{})
	scheme.AddKnownTypes(agentv1alpha1.GroupVersion, &agentv1alpha1.Agent{}, &agentv1alpha1.AgentList{})
	scheme.AddKnownTypes(logstashv1alpha1.GroupVersion, &logstashv1alpha1.Logstash{}, &logstashv1alpha1.LogstashList{})
	scheme.AddKnownTypes(rbacv1.SchemeGroupVersion, &rbacv1.ClusterRoleBinding{}, &rbacv1.ClusterRoleBindingList{})
	scheme.AddKnownTypes(rbacv1.SchemeGroupVersion, &rbacv1.ClusterRole{}, &rbacv1.ClusterRoleList{})
	scheme.AddKnownTypes(corev1.SchemeGroupVersion, &corev1.ServiceAccount{}, &corev1.ServiceAccountList{})
	scheme.AddKnownTypes(corev1.SchemeGroupVersion, &corev1.Service{}, &corev1.ServiceList{})
	scheme.AddKnownTypes(appsv1.SchemeGroupVersion, &appsv1.DaemonSet{})
	scheme.AddKnownTypes(corev1.SchemeGroupVersion, &corev1.Secret{}, &corev1.ConfigMap{})
	decoder := serializer.NewCodecFactory(scheme).UniversalDeserializer()

	return &YAMLDecoder{decoder: decoder}
}

func (yd *YAMLDecoder) ToBuilders(reader *bufio.Reader, transform BuilderTransform) ([]test.Builder, error) {
	var builders []test.Builder

	yamlReader := yaml.NewYAMLReader(reader)
	for {
		yamlBytes, err := yamlReader.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
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
		case *entv1.EnterpriseSearch:
			b := enterprisesearch.NewBuilderWithoutSuffix(decodedObj.Name)
			b.EnterpriseSearch = *decodedObj
			builder = transform(b)
		case *logstashv1alpha1.Logstash:
			b := logstash.NewBuilderWithoutSuffix(decodedObj.Name)
			b.Logstash = *decodedObj
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
			if errors.Is(err, io.EOF) {
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
//
//nolint:thelper
func RunFile(
	t *testing.T,
	filePath, namespace, suffix string,
	additionalObjects []client.Object,
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
) ([]test.Builder, []client.Object, error) {
	t.Helper()
	f, err := os.Open(filePath)
	require.NoError(t, err, "Failed to open file %s", filePath)
	defer f.Close()

	decoder := NewYAMLDecoder()
	objects, err := decoder.ToObjects(bufio.NewReader(f))
	if err != nil {
		return nil, nil, err
	}

	castObjects := make([]client.Object, len(objects))
	for i, obj := range objects {
		castObj, ok := obj.(client.Object)
		require.True(t, ok, "%T is not a client.Object", obj)
		castObjects[i] = castObj
	}

	builders, castObjects := transformToE2E(namespace, fullTestName, suffix, transformations, castObjects)
	return builders, castObjects, nil
}

//nolint:thelper
func makeObjectSteps(
	t *testing.T,
	objects []client.Object,
) (func(k *test.K8sClient) test.StepList, func(k *test.K8sClient) test.StepList) {
	//nolint:thelper
	return func(k *test.K8sClient) test.StepList {
			steps := test.StepList{}
			for i := range objects {
				ii := i
				meta, err := meta2.Accessor(objects[ii])
				require.NoError(t, err)
				steps = steps.WithStep(test.Step{
					Name: fmt.Sprintf("Create %s %s", objects[ii].GetObjectKind().GroupVersionKind().Kind, meta.GetName()),
					Test: test.Eventually(func() error {
						return k.CreateOrUpdate(objects[ii])
					}),
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
					Name: fmt.Sprintf("Delete %s %s", objects[ii].GetObjectKind().GroupVersionKind().Kind, meta.GetName()),
					Test: test.Eventually(func() error {
						err := k.Client.Delete(context.Background(), objects[ii])
						if err != nil && !apierrors.IsNotFound(err) {
							return err
						}
						return nil
					}),
				})
			}
			return steps
		}
}

func transformToE2E(namespace, fullTestName, suffix string, transformers []BuilderTransform, objects []client.Object) ([]test.Builder, []client.Object) {
	var builders []test.Builder
	var otherObjects []client.Object
	for _, object := range objects {
		var builder test.Builder
		switch decodedObj := object.(type) {
		case *esv1.Elasticsearch:
			b := elasticsearch.NewBuilderWithoutSuffix(decodedObj.Name)
			b.Elasticsearch = *decodedObj
			b = b.WithNamespace(namespace).
				WithSuffix(suffix).
				WithRestrictedSecurityContext().
				WithLabel(run.TestNameLabel, fullTestName).
				WithPodLabel(run.TestNameLabel, fullTestName)

			// for EKS, we set our e2e storage class to use local volumes instead of depending on the default storage class that uses
			// network storage because from k8s 1.23 network storage requires the installation of the Amazon EBS CSI driver and the
			// deployer does not yet support this. See https://github.com/elastic/cloud-on-k8s/issues/6515.
			if strings.HasPrefix(test.Ctx().Provider, "eks") {
				b = b.WithDefaultPersistentVolumes()
			}
			builder = b
		case *kbv1.Kibana:
			b := kibana.NewBuilderWithoutSuffix(decodedObj.Name)
			b.Kibana = *decodedObj
			builder = b.WithNamespace(namespace).
				WithSuffix(suffix).
				WithElasticsearchRef(tweakServiceRef(b.Kibana.Spec.ElasticsearchRef, suffix)).
				WithRestrictedSecurityContext().
				WithLabel(run.TestNameLabel, fullTestName).
				WithPodLabel(run.TestNameLabel, fullTestName).
				WithConfig(tweakConfigLiterals(b.Kibana.Spec.Config, suffix, namespace))
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
			b = b.WithNamespace(namespace).
				WithSuffix(suffix).
				WithElasticsearchRef(tweakServiceRef(b.Beat.Spec.ElasticsearchRef, suffix)).
				WithLabel(run.TestNameLabel, fullTestName).
				WithPodLabel(run.TestNameLabel, fullTestName).
				WithESValidations(beat.HasEventFromBeat(beatcommon.Type(b.Beat.Spec.Type))).
				WithKibanaRef(tweakServiceRef(b.Beat.Spec.KibanaRef, suffix))

			if b.PodTemplate.Spec.ServiceAccountName != "" {
				b = b.WithPodTemplateServiceAccount(b.PodTemplate.Spec.ServiceAccountName + "-" + suffix)
			}

			builder = b
		case *entv1.EnterpriseSearch:
			b := enterprisesearch.NewBuilderWithoutSuffix(decodedObj.Name)
			b.EnterpriseSearch = *decodedObj
			builder = b.WithNamespace(namespace).
				WithSuffix(suffix).
				WithElasticsearchRef(tweakServiceRef(b.EnterpriseSearch.Spec.ElasticsearchRef, suffix)).
				WithRestrictedSecurityContext().
				WithLabel(run.TestNameLabel, fullTestName).
				WithPodLabel(run.TestNameLabel, fullTestName)
		case *agentv1alpha1.Agent:
			b := agent.NewBuilderFromAgent(decodedObj)
			b = b.WithNamespace(namespace).
				WithSuffix(suffix).
				WithElasticsearchRefs(tweakOutputRefs(b.Agent.Spec.ElasticsearchRefs, suffix)...).
				WithLabel(run.TestNameLabel, fullTestName).
				WithPodLabel(run.TestNameLabel, fullTestName).
				WithKibanaRef(tweakServiceRef(b.Agent.Spec.KibanaRef, suffix)).
				WithFleetServerRef(tweakServiceRef(b.Agent.Spec.FleetServerRef, suffix))

			if b.PodTemplate.Spec.ServiceAccountName != "" {
				b = b.WithPodTemplateServiceAccount(b.PodTemplate.Spec.ServiceAccountName + "-" + suffix)
			}

			builder = b
		case *logstashv1alpha1.Logstash:
			b := logstash.NewBuilderWithoutSuffix(decodedObj.Name)

			esRefs := make([]logstashv1alpha1.ElasticsearchCluster, 0, len(b.Logstash.Spec.ElasticsearchRefs))
			for _, ref := range b.Logstash.Spec.ElasticsearchRefs {
				esRefs = append(esRefs, logstashv1alpha1.ElasticsearchCluster{
					ObjectSelector: tweakServiceRef(ref.ObjectSelector, suffix),
					ClusterName:    ref.ClusterName,
				})
			}

			b = b.WithNamespace(namespace).
				WithSuffix(suffix).
				WithElasticsearchRefs(esRefs...).
				WithLabel(run.TestNameLabel, fullTestName).
				WithPodLabel(run.TestNameLabel, fullTestName)

			builder = b
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
		case *corev1.Service:
			decodedObj.Namespace = namespace
			decodedObj.Name = decodedObj.Name + "-" + suffix
		case *appsv1.DaemonSet:
			name := decodedObj.Name + "-" + suffix
			decodedObj.Namespace = namespace
			decodedObj.Name = name
			decodedObj.Spec.Selector.MatchLabels["app.kubernetes.io/instance"] = name
			decodedObj.Spec.Template.ObjectMeta.Labels["app.kubernetes.io/instance"] = name
			maybeMutateForAgentNonRootTests(decodedObj, namespace, suffix)
		}

		if builder != nil {
			// ECK driven resources can be further transformed
			for _, transformer := range transformers {
				// This check is required as transformers is a variadic
				// argument to "RunFile" (not a slice) and sending nil will panic here.
				if transformer == nil {
					continue
				}
				builder = transformer(builder)
			}
			builders = append(builders, builder)
		} else {
			// built-in in resources are separated as they are treated differently
			otherObjects = append(otherObjects, object)
		}
	}

	sortBuilders(builders)

	return builders, otherObjects
}

// maybeMutateForAgentNonRootTests will possibly mutate the given daemonset when
// running tests for Elastic Agent running as non-root. This is required as the
// directories depend on both the namespace and the random suffix of the e2e tests.
func maybeMutateForAgentNonRootTests(ds *appsv1.DaemonSet, namespace, suffix string) {
	for i, init := range ds.Spec.Template.Spec.InitContainers {
		if init.Name == "manage-agent-hostpath-permissions" {
			for j, cmd := range ds.Spec.Template.Spec.InitContainers[i].Command {
				updatedCmd := strings.Replace(
					cmd,
					"/var/lib/elastic-agent/default/elastic-agent/state",
					fmt.Sprintf("/var/lib/elastic-agent/%s/elastic-agent-%s/state", namespace, suffix),
					1,
				)
				ds.Spec.Template.Spec.InitContainers[i].Command[j] = strings.Replace(
					updatedCmd,
					"/var/lib/elastic-agent/default/fleet-server/state",
					fmt.Sprintf("/var/lib/elastic-agent/%s/fleet-server-%s/state", namespace, suffix),
					1,
				)
			}
		}
	}
}

// sortBuilders mutates the given builder slice to sort them by test priority:
// Elasticsearch > Kibana >  APMServer > Enterprise Search > Beats
// The underlying goal is, for example, to ensure Elasticsearch is available before we start testing Beats.
func sortBuilders(builders []test.Builder) {
	sort.Slice(builders, func(i, j int) bool {
		return builderPriority(builders[i]) < builderPriority(builders[j])
	})
}

func builderPriority(builder test.Builder) int {
	switch builder.(type) {
	case elasticsearch.Builder:
		return 1
	case kibana.Builder:
		return 2
	case apmserver.Builder:
		return 3
	case enterprisesearch.Builder:
		return 4
	case beat.Builder:
		return 5
	default:
		return 100
	}
}

func tweakServiceRef(ref commonv1.ObjectSelector, suffix string) commonv1.ObjectSelector {
	// All the objects defined in the YAML file will have a random test suffix added to prevent clashes with previous runs.
	// This necessitates changing the Elasticsearch reference to match the suffixed name.
	if ref.Name != "" {
		ref.Name = ref.Name + "-" + suffix
	}

	return ref
}

func tweakOutputRefs(outputs []agentv1alpha1.Output, suffix string) (results []agentv1alpha1.Output) {
	for _, output := range outputs {
		// All the objects defined in the YAML file will have a random test suffix added to prevent clashes with previous runs.
		// This necessitates changing the Elasticsearch reference to match the suffixed name.
		ref := tweakServiceRef(output.ObjectSelector, suffix)
		output.ObjectSelector = ref
		results = append(results, output)
	}

	return results
}

func tweakConfigLiterals(config *commonv1.Config, suffix string, namespace string) map[string]interface{} {
	if config == nil {
		return map[string]interface{}{}
	}

	data := config.Data

	elasticsearchHostsKey := "xpack.fleet.agents.elasticsearch.hosts"
	if untypedHosts, ok := data[elasticsearchHostsKey]; ok {
		if untypedHostsSlice, ok := untypedHosts.([]interface{}); ok {
			for i, untypedHost := range untypedHostsSlice {
				if host, ok := untypedHost.(string); ok {
					untypedHostsSlice[i] = strings.ReplaceAll(
						host,
						"elasticsearch-es-http.default",
						fmt.Sprintf("elasticsearch-%s-es-http.%s", suffix, namespace),
					)
				}
			}
		}
	}

	fleetServerHostsKey := "xpack.fleet.agents.fleet_server.hosts"
	if untypedHosts, ok := data[fleetServerHostsKey]; ok {
		if untypedHostsSlice, ok := untypedHosts.([]interface{}); ok {
			for i, untypedHost := range untypedHostsSlice {
				if host, ok := untypedHost.(string); ok {
					untypedHostsSlice[i] = strings.ReplaceAll(
						host,
						"fleet-server-agent-http.default",
						fmt.Sprintf("fleet-server-%s-agent-http.%s", suffix, namespace),
					)
				}
			}
		}
	}

	fleetOutputsKey := "xpack.fleet.outputs"

	// This is only used when testing Agent+Fleet running as non-root. (config/recipes/elastic-agent/fleet-kubernetes-integration-nonroot.yaml)
	//
	// Adjust the Kibana's spec.config.xpack.fleet.outputs section to both
	// 1. Point to the valid Elasticsearch instance with suffix + namespace being random
	// 2. Point to the valid mounted Elasticsearch CA with a random suffix + namespace in the mount path.
	if untypedOutputs, ok := data[fleetOutputsKey]; ok { //nolint:nestif
		if untypedXpackOutputsSlice, ok := untypedOutputs.([]interface{}); ok {
			for _, untypedOutputMap := range untypedXpackOutputsSlice {
				if outputMap, ok := untypedOutputMap.(map[string]interface{}); ok {
					if outputMap["id"] == "eck-fleet-agent-output-elasticsearch" {
						if outputSlice, ok := outputMap["hosts"].([]interface{}); ok {
							for j, untypedHost := range outputSlice {
								if host, ok := untypedHost.(string); ok {
									outputSlice[j] = strings.ReplaceAll(
										host,
										"elasticsearch-es-http.default",
										fmt.Sprintf("elasticsearch-%s-es-http.%s", suffix, namespace),
									)
								}
							}
						}
						if untypedSSL, ok := outputMap["ssl"].(map[string]interface{}); ok {
							if untypedCAs, ok := untypedSSL["certificate_authorities"].([]interface{}); ok {
								for k, untypedCA := range untypedCAs {
									if ca, ok := untypedCA.(string); ok {
										untypedCAs[k] = strings.ReplaceAll(
											ca,
											"elasticsearch-association/default/elasticsearch/",
											fmt.Sprintf("elasticsearch-association/%s/elasticsearch-%s/", namespace, suffix),
										)
									}
								}
							}
						}
					}
				}
			}
		}
	}

	return data
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
