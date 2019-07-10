// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package version6

import (
	"bytes"
	"io"
	"io/ioutil"
	"net/http"
	"strconv"
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	common "github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/mutation"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/pod"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func fakeEsClient(raiseError bool) client.Client {
	return client.NewMockClient(version.MustParse("6.7.0"), func(req *http.Request) *http.Response {
		var statusCode int
		var respBody io.ReadCloser

		if raiseError {
			respBody = ioutil.NopCloser(bytes.NewBufferString("KO"))
			statusCode = 400
		} else {
			respBody = ioutil.NopCloser(bytes.NewBufferString("OK"))
			statusCode = 200
		}

		return &http.Response{
			StatusCode: statusCode,
			Body:       respBody,
			Header:     make(http.Header),
			Request:    req,
		}
	})
}

func newMasterPod(name, namespace, ssetName string) corev1.Pod {
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				string(label.NodeTypesMasterLabelName): strconv.FormatBool(true),
				label.StatefulSetNameLabelName:         ssetName,
			},
		},
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{
				{
					Type:   corev1.ContainersReady,
					Status: corev1.ConditionTrue,
				},
				{
					Type:   corev1.ContainersReady,
					Status: corev1.ConditionTrue,
				},
			},
		},
	}
	return pod
}

func ssetConfig(namespace, ssetName string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      settings.ConfigSecretName(ssetName),
		},
		Data: map[string][]byte{
			settings.ConfigFileName: []byte("a: b\nc: d\n"),
		},
	}
}
func setupScheme(t *testing.T) *runtime.Scheme {
	sc := scheme.Scheme
	if err := v1alpha1.SchemeBuilder.AddToScheme(sc); err != nil {
		assert.Fail(t, "failed to add Es types")
	}
	return sc
}

func TestUpdateZen1Discovery(t *testing.T) {
	s := setupScheme(t)
	cluster := v1alpha1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns1",
			Name:      "es1",
		},
	}
	ssetName := "master-nodes"
	type args struct {
		cluster            v1alpha1.Elasticsearch
		c                  k8s.Client
		esClient           client.Client
		allPods            []corev1.Pod
		performableChanges *mutation.PerformableChanges
		state              *reconcile.State
	}
	tests := []struct {
		name                      string
		args                      args
		want                      bool
		expectedMinimumMasterNode string
		wantErr                   bool
	}{
		{
			name: "Update a one master node cluster",
			args: args{
				esClient: fakeEsClient(true), // second master is not created, raise an error if API is called
				c:        k8s.WrapClient(fake.NewFakeClientWithScheme(s, ssetConfig("ns1", ssetName))),
				performableChanges: &mutation.PerformableChanges{
					Changes: mutation.Changes{
						ToCreate: []mutation.PodToCreate{
							{
								Pod: newMasterPod("master2", "ns1", ssetName),
								PodSpecCtx: pod.PodSpecContext{
									Config: settings.CanonicalConfig{CanonicalConfig: common.NewCanonicalConfig()},
								},
							},
						},
					},
				},
				allPods: []corev1.Pod{
					newMasterPod("master1", "ns1", ssetName),
				},
				state: reconcile.NewState(cluster),
			},
			want:                      true,
			wantErr:                   false,
			expectedMinimumMasterNode: "2",
		},
		{
			name: "Add a master to a four master node cluster",
			args: args{
				esClient: fakeEsClient(false), // a majority of master is available, call the API
				c: k8s.WrapClient(fake.NewFakeClientWithScheme(
					s,
					ssetConfig("ns1", ssetName),
				),
				),
				performableChanges: &mutation.PerformableChanges{
					Changes: mutation.Changes{
						ToCreate: []mutation.PodToCreate{
							{
								Pod: newMasterPod("master5", "ns1", ssetName),
								PodSpecCtx: pod.PodSpecContext{
									Config: settings.CanonicalConfig{CanonicalConfig: common.NewCanonicalConfig()},
								},
							},
						},
					},
				},
				allPods: []corev1.Pod{
					newMasterPod("master1", "ns1", ssetName),
					newMasterPod("master2", "ns1", ssetName),
					newMasterPod("master3", "ns1", ssetName),
					newMasterPod("master4", "ns1", ssetName),
				},
				state: reconcile.NewState(cluster),
			},
			want:                      false, // mmn should also be updated with the API
			wantErr:                   false,
			expectedMinimumMasterNode: "3",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := UpdateZen1Discovery(
				tt.args.cluster,
				tt.args.c,
				tt.args.esClient,
				tt.args.allPods,
				tt.args.performableChanges,
				tt.args.state,
			)
			if (err != nil) != tt.wantErr {
				t.Errorf("UpdateZen1Discovery() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("UpdateZen1Discovery() = %v, want %v", got, tt.want)
			}
			// Check the mmn in the new pods
			for _, newPod := range tt.args.performableChanges.ToCreate {
				expectedConfiguration :=
					common.MustNewSingleValue(settings.DiscoveryZenMinimumMasterNodes, tt.expectedMinimumMasterNode)
				if diff := newPod.PodSpecCtx.Config.Diff(expectedConfiguration, nil); diff != nil {
					t.Errorf("zen1.UpdateZen1Discovery() = %v, want %v", diff, tt.want)
				}
			}
			if !tt.want { // requeue not returned: it means that minimum_master_nodes should be saved in status
				assert.Equal(t, tt.expectedMinimumMasterNode, strconv.Itoa(tt.args.state.GetZen1MinimumMasterNodes()))
			}
		})
	}
}
