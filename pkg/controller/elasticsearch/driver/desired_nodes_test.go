// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package driver

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/utils/pointer"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	common "github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

func Test_defaultDriver_updateDesiredNodes(t *testing.T) {
	type args struct {
		esReachable bool
	}
	type wantCondition struct {
		status  corev1.ConditionStatus
		message string
	}
	type want struct {
		testdata     string // expected captured request
		deleteCalled bool
		results      *reconciler.Results
		condition    *wantCondition
	}
	tests := []struct {
		name      string
		esBuilder esBuilder
		args      args
		want      want
	}{
		{
			name: "Happy path",
			args: args{
				esReachable: true,
			},
			esBuilder: newEs("8.3.0").
				withNodeSet(
					nodeSet("master", 3).
						withCPU("2222m", "3141m").
						withMemory("2333Mi", "2333Mi").
						withStorage("1Gi", "1Gi").pvcCreated(true).
						withNodeCfg(map[string]interface{}{
							"node.roles":              []string{"master"},
							"node.name":               "${POD_NAME}",
							"path.data":               "/usr/share/elasticsearch/data",
							"network.publish_host":    "${POD_IP}",
							"http.publish_host":       "${POD_NAME}.${HEADLESS_SERVICE_NAME}.${NAMESPACE}.svc",
							"node.attr.k8s_node_name": "${NODE_NAME}",
						}),
				).withNodeSet(
				nodeSet("hot", 3).
					withCPU("", "1"). // Setting only limits if fine.
					withMemory("", "4Gi").
					withStorage("10Gi", "50Gi").pvcCreated(true).
					withNodeCfg(map[string]interface{}{
						"node.roles":              []string{"data", "ingest"},
						"node.name":               "${POD_NAME}",
						"path.data":               "/usr/share/elasticsearch/data",
						"network.publish_host":    "${POD_IP}",
						"http.publish_host":       "${POD_NAME}.${HEADLESS_SERVICE_NAME}.${NAMESPACE}.svc",
						"node.attr.k8s_node_name": "${NODE_NAME}",
					}),
			),
			want: want{
				results:  &reconciler.Results{},
				testdata: "happy_path.json",
			},
		},
		{
			name: "No PVC yet",
			args: args{
				esReachable: true,
			},
			esBuilder: newEs("8.3.0").
				withNodeSet(
					nodeSet("master", 3).
						withCPU("2222m", "3141m").
						withMemory("2333Mi", "2333Mi").
						withStorage("1Gi", "").pvcCreated(false).
						withNodeCfg(map[string]interface{}{
							"node.roles":              []string{"master"},
							"node.name":               "${POD_NAME}",
							"path.data":               "/usr/share/elasticsearch/data",
							"network.publish_host":    "${POD_IP}",
							"http.publish_host":       "${POD_NAME}.${HEADLESS_SERVICE_NAME}.${NAMESPACE}.svc",
							"node.attr.k8s_node_name": "${NODE_NAME}",
						}),
				).withNodeSet(
				nodeSet("hot", 3).
					withCPU("", "1").
					withMemory("", "4Gi").
					withStorage("50Gi", "").pvcCreated(false).
					withNodeCfg(map[string]interface{}{
						"node.roles":              []string{"data", "ingest"},
						"node.name":               "${POD_NAME}",
						"path.data":               "/usr/share/elasticsearch/data",
						"network.publish_host":    "${POD_IP}",
						"http.publish_host":       "${POD_NAME}.${HEADLESS_SERVICE_NAME}.${NAMESPACE}.svc",
						"node.attr.k8s_node_name": "${NODE_NAME}",
					}),
			),
			want: want{
				results: (&reconciler.Results{}).
					WithReconciliationState(defaultRequeue.
						WithReason("Storage capacity is not available in all PVC statuses, requeue to refine the capacity reported in the desired nodes API"),
					),
				testdata: "happy_path.json",
				condition: &wantCondition{
					status:  corev1.ConditionTrue,
					message: "Successfully calculated compute and storage resources from Elasticsearch resource generation ",
				},
			},
		},
		{
			name: "No capacity in PVC status",
			args: args{
				esReachable: true,
			},
			esBuilder: newEs("8.3.0").
				withNodeSet(
					nodeSet("master", 3).
						withCPU("2222m", "3141m").
						withMemory("2333Mi", "2333Mi").
						withStorage("1Gi", "").pvcCreated(true).
						withNodeCfg(map[string]interface{}{
							"node.roles":              []string{"master"},
							"node.name":               "${POD_NAME}",
							"path.data":               "/usr/share/elasticsearch/data",
							"network.publish_host":    "${POD_IP}",
							"http.publish_host":       "${POD_NAME}.${HEADLESS_SERVICE_NAME}.${NAMESPACE}.svc",
							"node.attr.k8s_node_name": "${NODE_NAME}",
						}),
				).withNodeSet(
				nodeSet("hot", 3).
					withCPU("", "1").
					withMemory("", "4Gi").
					withStorage("50Gi", "").pvcCreated(true).
					withNodeCfg(map[string]interface{}{
						"node.roles":              []string{"data", "ingest"},
						"node.name":               "${POD_NAME}",
						"path.data":               "/usr/share/elasticsearch/data",
						"network.publish_host":    "${POD_IP}",
						"http.publish_host":       "${POD_NAME}.${HEADLESS_SERVICE_NAME}.${NAMESPACE}.svc",
						"node.attr.k8s_node_name": "${NODE_NAME}",
					}),
			),
			want: want{
				results: (&reconciler.Results{}).
					WithReconciliationState(defaultRequeue.
						WithReason("Storage capacity is not available in all PVC statuses, requeue to refine the capacity reported in the desired nodes API"),
					),
				testdata: "happy_path.json",
				condition: &wantCondition{
					status:  corev1.ConditionTrue,
					message: "Successfully calculated compute and storage resources from Elasticsearch resource generation ",
				},
			},
		},
		{
			name: "Multi data path",
			args: args{
				esReachable: true,
			},
			esBuilder: newEs("8.3.0").
				withNodeSet(
					nodeSet("default", 3).
						withCPU("2", "4").
						withMemory("2Gi", "2Gi").
						withStorage("1Gi", "1Gi").
						withNodeCfg(map[string]interface{}{
							"path.data": []string{"/usr/share/elasticsearch/data1", "/usr/share/elasticsearch/data2"},
						}),
				),
			want: want{
				results:      &reconciler.Results{},
				deleteCalled: true,
				condition: &wantCondition{
					status:  corev1.ConditionFalse,
					message: "Elasticsearch path.data must be a string, multiple paths is not supported",
				},
			},
		},
		{
			name: "No memory limit",
			args: args{
				esReachable: true,
			},
			esBuilder: newEs("8.3.0").
				withNodeSet(
					nodeSet("default", 3).
						withCPU("2222m", "3141m").
						withMemory("2333Mi", "").
						withStorage("1Gi", "1Gi"),
				),
			want: want{
				results:      &reconciler.Results{},
				deleteCalled: true,
				condition: &wantCondition{
					status:  corev1.ConditionFalse,
					message: "memory limit is not set",
				},
			},
		},
		{
			name: "Memory request and limit should be the same",
			args: args{
				esReachable: true,
			},
			esBuilder: newEs("8.3.0").
				withNodeSet(
					nodeSet("default", 3).
						withCPU("2222m", "3141m").
						withMemory("2333Mi", "2334Mi").
						withStorage("1Gi", "1Gi"),
				),
			want: want{
				results:      &reconciler.Results{},
				deleteCalled: true,
				condition: &wantCondition{
					status:  corev1.ConditionFalse,
					message: "memory request and limit do not have the same value",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			existingResources := tt.esBuilder.toResources()
			es := tt.esBuilder.toEs()
			reconcileState, err := reconcile.NewState(es)
			if err != nil {
				assert.FailNow(t, "fatal: %s", err)
			}
			k8sClient := k8s.NewFakeClient(existingResources...)
			d := &defaultDriver{
				DefaultDriverParameters: DefaultDriverParameters{
					ReconcileState: reconcileState,
					ES:             es,        // TODO: duplicate
					Client:         k8sClient, // TODO: duplicate
				},
			}
			expectedResources := tt.esBuilder.toExpectedResources()

			wantClient := wantClient{}
			if tt.want.testdata != "" {
				parsedRequest, err := ioutil.ReadFile("testdata/desired_nodes/" + tt.want.testdata)
				assert.NoError(t, err)
				wantClient.request = string(parsedRequest)
				// Elasticsearch UID must have been used as the history ID
				wantClient.historyID = es.UID
				// Elasticsearch generation must have been used as the version
				wantClient.version = es.Generation
			}

			esClient := fakeEsClient(t, "8.3.0", false, wantClient)
			if got := d.updateDesiredNodes(context.TODO(), k8sClient, esClient, tt.args.esReachable, expectedResources); !reflect.DeepEqual(*got, *tt.want.results) {
				t.Errorf("defaultDriver.updateDesiredNodes() = %v, want %v", *got, *tt.want.results)
			}
			assert.Equal(t, tt.want.deleteCalled, esClient.deleted)

			// Check that the status has been updated accordingly.
			if tt.want.condition == nil {
				return
			}
			condition := d.ReconcileState.Index(esv1.ResourcesAwareManagement)
			hasCondition := condition >= 0
			assert.True(t, hasCondition, "ResourcesAwareManagement condition should be set")
			if hasCondition {
				c := d.ReconcileState.Conditions[condition]
				assert.Equal(t, tt.want.condition.status, c.Status)
				assert.True(t, strings.Contains(c.Message, tt.want.condition.message), "expected message in condition: \"%s\", got \"%s\" ", tt.want.condition.message, c.Message)
			}
		})
	}
}

// -- Fixtures and helpers --

type esBuilder struct {
	esVersion       string
	nodeSets        []fakeNodeSet
	uid             types.UID
	resourceVersion string
}

func (esb esBuilder) toExpectedResources() nodespec.ResourcesList {
	resources := make([]nodespec.Resources, len(esb.nodeSets))
	for i := range esb.nodeSets {
		fns := esb.nodeSets[i]
		ssetname := esv1.StatefulSet("elasticsearch-desired-sample", fns.name)
		resources[i] = nodespec.Resources{
			NodeSet:         fns.name,
			HeadlessService: nodespec.HeadlessService(&es, ssetname),
			Config:          settings.CanonicalConfig{CanonicalConfig: common.MustCanonicalConfig(fns.nodeConfig)},
			StatefulSet: v1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ssetname,
					Namespace: "default",
				},
				Spec: v1.StatefulSetSpec{
					Replicas: pointer.Int32(fns.count),
					Template: fns.toPodTemplateSpec(),
					VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
						{
							ObjectMeta: metav1.ObjectMeta{Name: "elasticsearch-data"},
							Spec: corev1.PersistentVolumeClaimSpec{
								Resources: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceStorage: fns.claimedStorage.DeepCopy(),
									},
								},
							},
						},
					},
				},
			},
		}
	}
	return resources
}

func (esb esBuilder) toEs() esv1.Elasticsearch {
	nodeSets := make([]esv1.NodeSet, len(esb.nodeSets))
	for i := range esb.nodeSets {
		fns := esb.nodeSets[i]
		nodeSets[i].Name = fns.name
		nodeSets[i].PodTemplate = fns.toPodTemplateSpec()
		nodeSets[i].Config = &commonv1.Config{Data: fns.nodeConfig}
		nodeSets[i].Count = fns.count
	}

	return esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "elasticsearch-desired-sample",
			Namespace:       "default",
			UID:             esb.uid,
			ResourceVersion: esb.resourceVersion,
			Generation:      1,
		},
		Spec: esv1.ElasticsearchSpec{
			Version:  esb.esVersion,
			NodeSets: nodeSets,
		},
	}
}

func (esb esBuilder) toResources() []runtime.Object {
	es := esb.toEs()
	result := []runtime.Object{&es}
	for _, nodeSet := range esb.nodeSets {
		if !nodeSet.pvcExists {
			continue
		}
		for i := 0; i < int(nodeSet.count); i++ {
			pvc := corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:            fmt.Sprintf("elasticsearch-data-elasticsearch-desired-sample-es-%s-%d", nodeSet.name, i),
					Namespace:       "default",
					UID:             uuid.NewUUID(),
					ResourceVersion: strconv.Itoa(rand.Intn(1000)),
					Generation:      1,
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{corev1.ResourceStorage: nodeSet.claimedStorage.DeepCopy()},
					},
				},
			}
			if nodeSet.storageInStatus != nil {
				pvc.Status = corev1.PersistentVolumeClaimStatus{
					Capacity: corev1.ResourceList{corev1.ResourceStorage: nodeSet.storageInStatus.DeepCopy()},
				}
			}
			result = append(result, &pvc)
		}
	}
	return result
}

func newEs(esVersion string) esBuilder {
	return esBuilder{
		esVersion:       esVersion,
		uid:             uuid.NewUUID(),
		resourceVersion: strconv.Itoa(rand.Intn(1000)),
	}
}

func (esb esBuilder) withNodeSet(fn fakeNodeSet) esBuilder {
	esb.nodeSets = append(esb.nodeSets, fn)
	return esb
}

type fakeNodeSet struct {
	name                       string
	count                      int32
	nodeConfig                 map[string]interface{}
	cpuRequest, cpuLimit       *resource.Quantity
	memoryRequest, memoryLimit *resource.Quantity

	pvcExists                       bool
	claimedStorage, storageInStatus *resource.Quantity
}

func (fn fakeNodeSet) toPodTemplateSpec() corev1.PodTemplateSpec {
	// Build the resources
	resources := corev1.ResourceRequirements{
		Limits:   make(map[corev1.ResourceName]resource.Quantity),
		Requests: make(map[corev1.ResourceName]resource.Quantity),
	}
	if fn.cpuRequest != nil {
		resources.Requests[corev1.ResourceCPU] = fn.cpuRequest.DeepCopy()
	}
	if fn.cpuLimit != nil {
		resources.Limits[corev1.ResourceCPU] = fn.cpuLimit.DeepCopy()
	}
	if fn.memoryRequest != nil {
		resources.Requests[corev1.ResourceMemory] = fn.memoryRequest.DeepCopy()
	}
	if fn.memoryLimit != nil {
		resources.Limits[corev1.ResourceMemory] = fn.memoryLimit.DeepCopy()
	}

	esContainer := corev1.Container{
		Name:      "elasticsearch",
		Resources: resources,
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "elasticsearch-data",
				MountPath: "/usr/share/elasticsearch/data",
			},
		},
	}

	return corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{esContainer},
		},
	}
}

// nodeSet returns a fake nodeSet builder with a given name and a given size.
func nodeSet(name string, count int32) fakeNodeSet {
	return fakeNodeSet{
		name:  name,
		count: count,
	}
}

func (fn fakeNodeSet) withNodeCfg(cfg map[string]interface{}) fakeNodeSet {
	fn.nodeConfig = cfg
	return fn
}

func (fn fakeNodeSet) withCPU(request, limit string) fakeNodeSet {
	fn.cpuRequest, fn.cpuLimit = parseQuantityStrings(request, limit)
	return fn
}

func (fn fakeNodeSet) withMemory(request, limit string) fakeNodeSet {
	fn.memoryRequest, fn.memoryLimit = parseQuantityStrings(request, limit)
	return fn
}

func (fn fakeNodeSet) pvcCreated(created bool) fakeNodeSet {
	fn.pvcExists = created
	return fn
}

func (fn fakeNodeSet) withStorage(claimed, inStatus string) fakeNodeSet {
	fn.claimedStorage, fn.storageInStatus = parseQuantityStrings(claimed, inStatus)
	return fn
}

func parseQuantityStrings(request, limit string) (requestQuantity *resource.Quantity, requestLimit *resource.Quantity) {
	if request != "" {
		q := resource.MustParse(request)
		requestQuantity = &q
	}
	if limit != "" {
		q := resource.MustParse(limit)
		requestLimit = &q
	}
	return
}

type desiredNodesFakeClient struct {
	client.Client
	deleted bool
}

func (c *desiredNodesFakeClient) UpdateDesiredNodes(ctx context.Context, historyID string, version int64, desiredNodes client.DesiredNodes) error {
	return c.Client.UpdateDesiredNodes(ctx, historyID, version, desiredNodes)
}

func (c *desiredNodesFakeClient) DeleteDesiredNodes(ctx context.Context) error {
	c.deleted = true
	return c.Client.DeleteDesiredNodes(ctx)
}

type wantClient struct {
	historyID types.UID
	version   int64
	request   string
}

const expectedPath = `^\/_internal\/desired_nodes\/(?P<history>.*)\/(?P<version>.*)$`

func fakeEsClient(t *testing.T, esVersion string, err bool, want wantClient) *desiredNodesFakeClient {
	t.Helper()
	expectedPath := regexp.MustCompile(expectedPath)
	c := client.NewMockClient(version.MustParse(esVersion), func(req *http.Request) *http.Response {
		if !strings.HasPrefix(req.URL.Path, "/_internal/desired_nodes") {
			t.Fatalf("Elasticsearch client has been called on unknown path: %s", req.URL.Path)
		}
		statusCode := 200
		if err {
			statusCode = 500
		}

		var (
			gotHistoryID types.UID
			gotVersion   int64
		)
		match := expectedPath.FindStringSubmatch(req.URL.Path)
		if len(match) > 2 {
			gotHistoryID = types.UID(match[1])
			parsedVersion, err := strconv.ParseInt(match[2], 10, 64)
			assert.NoError(t, err)
			gotVersion = parsedVersion
		}

		// Compare history and version
		assert.Equal(t, want.historyID, gotHistoryID)
		assert.Equal(t, want.version, gotVersion)

		// Compare the request
		if want.request != "" {
			gotRequest, err := ioutil.ReadAll(req.Body)
			assert.NoError(t, err)
			require.JSONEq(t, want.request, string(gotRequest))
		}

		return &http.Response{
			StatusCode: statusCode,
			Body:       ioutil.NopCloser(bytes.NewBufferString(`{"acknowledged":true}`)),
		}
	})
	return &desiredNodesFakeClient{Client: c}
}
