// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package driver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/utils/ptr"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	common "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/hints"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

func Test_defaultDriver_updateDesiredNodes(t *testing.T) {
	type args struct {
		esReachable       bool
		esClientError     bool
		k8sClientError    bool
		orchestrationHint *hints.DesiredNodesHint
	}
	type wantCondition struct {
		status   corev1.ConditionStatus
		messages []string
	}
	type wantResult struct {
		error        bool
		reason       string
		requeue      bool
		requeueAfter time.Duration
	}
	type want struct {
		testdata     string // expected captured request
		deleteCalled bool
		result       wantResult
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
					withCPU("", "1"). // Setting only limits is also fine.
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
				result:   wantResult{},
				testdata: "happy_path.json",
				condition: &wantCondition{
					status:   corev1.ConditionTrue,
					messages: []string{"Successfully calculated compute and storage resources from Elasticsearch resource generation "},
				},
			},
		},
		{
			name: "With orchestration hint",
			args: args{
				esReachable: true,
				orchestrationHint: &hints.DesiredNodesHint{
					Version: 123,
					Hash:    "2328297597",
				},
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
					withCPU("", "1"). // Setting only limits is also fine.
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
				result: wantResult{},
				// no request expected here
				condition: &wantCondition{
					status:   corev1.ConditionTrue,
					messages: []string{"Successfully calculated compute and storage resources from Elasticsearch resource generation "},
				},
			},
		},
		{
			name: "Discard prerelease and build number",
			args: args{
				esReachable: true,
			},
			esBuilder: newEs("8.3.0-SNAPSHOT").
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
					withCPU("", "1"). // Setting only limits is also fine.
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
				result:   wantResult{},
				testdata: "happy_path.json",
				condition: &wantCondition{
					status:   corev1.ConditionTrue,
					messages: []string{"Successfully calculated compute and storage resources from Elasticsearch resource generation "},
				},
			},
		},
		{
			name: "Expected resources are calculated but Elasticsearch is not reachable",
			args: args{
				esReachable: false,
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
					withCPU("", "1").
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
				result: wantResult{
					requeueAfter: defaultRequeue.RequeueAfter,
					requeue:      defaultRequeue.Requeue,
					reason:       "Waiting for Elasticsearch to be available to update the desired nodes API",
				},
				condition: &wantCondition{
					status:   corev1.ConditionTrue,
					messages: []string{"Successfully calculated compute and storage resources from Elasticsearch resource generation "},
				},
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
						withStorage("1Gi", "").
						pvcCreated(false). // PVC does not exist yet
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
					withStorage("50Gi", "").
					pvcCreated(false). // PVC does not exist yet
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
				result: wantResult{
					requeueAfter: defaultRequeue.RequeueAfter,
					requeue:      defaultRequeue.Requeue, // requeue is expected to get a more accurate storage capacity from the PVC status later
					reason:       "Storage capacity is not available in all PVC statuses, requeue to refine the capacity reported in the desired nodes API",
				},
				testdata: "happy_path.json",
				condition: &wantCondition{
					status:   corev1.ConditionTrue,
					messages: []string{"Successfully calculated compute and storage resources from Elasticsearch resource generation "},
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
						withStorage("1Gi", "" /* No capacity in PVC status */).pvcCreated(true).
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
					withStorage("50Gi", "" /* No capacity in PVC status */).pvcCreated(true).
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
				result: wantResult{
					requeueAfter: defaultRequeue.RequeueAfter,
					requeue:      defaultRequeue.Requeue, // requeue is expected to get a more accurate storage capacity from the PVC status later
					reason:       "Storage capacity is not available in all PVC statuses, requeue to refine the capacity reported in the desired nodes API",
				},
				testdata: "happy_path.json",
				condition: &wantCondition{
					status:   corev1.ConditionTrue,
					messages: []string{"Successfully calculated compute and storage resources from Elasticsearch resource generation "},
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
				result:       wantResult{},
				deleteCalled: true,
				condition: &wantCondition{
					status:   corev1.ConditionFalse,
					messages: []string{"Elasticsearch path.data must be a string, multiple paths is not supported"},
				},
			},
		},
		{
			name: "Elasticsearch client returned an error",
			args: args{
				esReachable:   true,
				esClientError: true,
			},
			esBuilder: newEs("8.3.0").
				withNodeSet(
					nodeSet("default", 3).
						withCPU("2", "4").
						withMemory("2Gi", "2Gi").
						withStorage("1Gi", "1Gi").
						pvcCreated(true).
						withNodeCfg(map[string]interface{}{
							"path.data": "/usr/share/elasticsearch/data",
						}),
				),
			want: want{
				result: wantResult{
					error:  true,
					reason: "elasticsearch client failed",
				},
				condition: &wantCondition{
					status:   corev1.ConditionTrue,
					messages: []string{"Successfully calculated compute and storage resources from Elasticsearch resource generation "},
				},
			},
		},
		{
			name: "Kubernetes client returned an error",
			args: args{
				esReachable:    true,
				k8sClientError: true,
			},
			esBuilder: newEs("8.3.0").
				withNodeSet(
					nodeSet("default", 3).
						withCPU("2", "4").
						withMemory("2Gi", "2Gi").
						withStorage("1Gi", "1Gi").
						pvcCreated(true).
						withNodeCfg(map[string]interface{}{
							"path.data": "/usr/share/elasticsearch/data",
						}),
				),
			want: want{
				result: wantResult{
					error:  true,
					reason: "k8s client failed",
				},
				condition: &wantCondition{
					status:   corev1.ConditionUnknown,
					messages: []string{"Error while calculating compute and storage resources from Elasticsearch resource generation "},
				},
			},
		},
		{
			name: "0 values are not allowed",
			args: args{
				esReachable: true,
			},
			esBuilder: newEs("8.3.0").
				withNodeSet(
					nodeSet("default", 3).
						withCPU("0", "").
						withMemory("0", "0").
						withStorage("1Gi", "1Gi").
						withNodeCfg(map[string]interface{}{
							"path.data": "/usr/share/elasticsearch/data",
						}),
				),
			want: want{
				result:       wantResult{},
				deleteCalled: true,
				condition: &wantCondition{
					status: corev1.ConditionFalse,
					messages: []string{
						"CPU request is set but value is 0",
						"memory limit is set but value is 0",
					},
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
				result:       wantResult{},
				deleteCalled: true,
				condition: &wantCondition{
					status:   corev1.ConditionFalse,
					messages: []string{"memory limit is not set"},
				},
			},
		},
		{
			name: "Cannot compute resources and es is not reachable: requeue is expected",
			args: args{
				esReachable: false,
			},
			esBuilder: newEs("8.3.0").
				withNodeSet(
					nodeSet("default", 3).
						withCPU("2222m", "3141m").
						withMemory("2333Mi", "").
						withStorage("1Gi", "1Gi"),
				),
			want: want{
				result: wantResult{
					requeue:      true,
					requeueAfter: defaultRequeue.RequeueAfter,
				},
				deleteCalled: false, // Elasticsearch is not reachable, client cannot be called
				condition: &wantCondition{
					status:   corev1.ConditionFalse,
					messages: []string{"memory limit is not set"},
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
				result:       wantResult{},
				deleteCalled: true,
				condition: &wantCondition{
					status:   corev1.ConditionFalse,
					messages: []string{"memory request and limit do not have the same value"},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			es := tt.esBuilder.toEs()
			reconcileState, err := reconcile.NewState(es)
			if err != nil {
				assert.FailNow(t, "fatal: %s", err)
			}

			if tt.args.orchestrationHint != nil {
				reconcileState.UpdateOrchestrationHints(hints.OrchestrationsHints{DesiredNodes: tt.args.orchestrationHint})
			}

			var k8sClient k8s.Client

			if tt.args.k8sClientError {
				k8sClient = k8s.NewFailingClient(errors.New("k8s client failed"))
			} else {
				existingResources := tt.esBuilder.toResources()
				k8sClient = k8s.NewFakeClient(existingResources...)
			}

			d := &defaultDriver{
				DefaultDriverParameters: DefaultDriverParameters{
					ReconcileState: reconcileState,
					ES:             es,
					Client:         k8sClient,
				},
			}

			wantClient := wantClient{}
			if tt.want.testdata != "" {
				parsedRequest, err := os.ReadFile("testdata/desired_nodes/" + tt.want.testdata)
				assert.NoError(t, err)
				wantClient.request = string(parsedRequest)
				// Elasticsearch UID must have been used as the history ID
				wantClient.historyID = es.UID
			}

			esClient := fakeEsClient(t, "8.3.0", tt.args.esClientError, wantClient)
			got := d.updateDesiredNodes(context.TODO(), esClient, tt.args.esReachable, tt.esBuilder.toExpectedResources())

			// Check reconcile result
			result, err := got.Aggregate()
			assert.Equal(t, tt.want.result.error, err != nil, "updateDesiredNodes(...): unexpected error result")
			assert.Equal(t, tt.want.result.requeue, result.Requeue, "updateDesiredNodes(...): unexpected requeue result")
			assert.Equal(t, tt.want.result.requeueAfter, result.RequeueAfter, "updateDesiredNodes(...): unexpected result.RequeueAfter value")
			_, gotReason := got.IsReconciled()
			assert.True(t, strings.Contains(gotReason, tt.want.result.reason), "updateDesiredNodes(...): unexpected reconciled reason")

			// Check if the Elasticsearch client has been called as expected
			assert.Equal(t, tt.want.deleteCalled, esClient.deleted)

			// Check that the status has been updated accordingly.
			if tt.want.condition == nil {
				return
			}
			condition := d.ReconcileState.Index(esv1.ResourcesAwareManagement)
			hasCondition := condition >= 0
			assert.True(t, hasCondition, "ResourcesAwareManagement condition should be set")
			if !hasCondition {
				return
			}
			c := d.ReconcileState.Conditions[condition]
			assert.Equal(t, tt.want.condition.status, c.Status)
			for _, expectedMessage := range tt.want.condition.messages {
				assert.True(t, strings.Contains(c.Message, expectedMessage), "expected message in condition: %q, got %q", expectedMessage, c.Message)
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
					Replicas: ptr.To[int32](fns.count),
					Template: fns.toPodTemplateSpec(),
					VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
						{
							ObjectMeta: metav1.ObjectMeta{Name: "elasticsearch-data"},
							Spec: corev1.PersistentVolumeClaimSpec{
								Resources: corev1.VolumeResourceRequirements{
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

func (esb esBuilder) toResources() []crclient.Object {
	es := esb.toEs()
	result := []crclient.Object{&es}
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
					Resources: corev1.VolumeResourceRequirements{
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
		desiredNodesVersion := int64(123)
		statusCode := 200
		if err {
			statusCode = 500
		}
		if req.Method == http.MethodGet {
			resp, err := json.Marshal(client.LatestDesiredNodes{Version: desiredNodesVersion})
			require.NoError(t, err)
			return &http.Response{
				StatusCode: statusCode,
				Body:       io.NopCloser(bytes.NewReader(resp)),
			}
		}

		if want.request != "" {
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
			// we only ever test one iteration of desired nodes updates so simply increment the version from _latest by one
			assert.Equal(t, desiredNodesVersion+1, gotVersion)

			// Compare the request
			gotRequest, err := io.ReadAll(req.Body)
			assert.NoError(t, err)
			require.JSONEq(t, want.request, string(gotRequest))
		} else if req.Method != http.MethodDelete { // delete is captured through the fake ES client
			t.Fatalf("Unexpected request %s %s", req.Method, req.URL.Path)
		}

		return &http.Response{
			StatusCode: statusCode,
			Body:       io.NopCloser(bytes.NewBufferString(`{"acknowledged":true}`)),
		}
	})
	return &desiredNodesFakeClient{Client: c}
}
