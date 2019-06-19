// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package discovery

import (
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/name"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/volume"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// newPodWithIP creates a new Pod potentially labeled as master with a given podIP
func newPodWithIP(name, ip string, master bool) corev1.Pod {
	p := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: make(map[string]string),
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{}},
		},
		Status: corev1.PodStatus{
			PodIP: ip,
		},
	}
	label.NodeTypesMasterLabelName.Set(master, p.Labels)
	return p
}

func TestUpdateSeedHostsConfigMap(t *testing.T) {
	require.NoError(t, v1alpha1.AddToScheme(scheme.Scheme))
	es := v1alpha1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "es1",
			Namespace: "ns1",
		},
	}
	type args struct {
		c      k8s.Client
		scheme *runtime.Scheme
		es     v1alpha1.Elasticsearch
		pods   []corev1.Pod
	}
	tests := []struct {
		name            string
		args            args
		wantErr         bool
		expectedContent string
	}{
		{
			name: "Do not fail if no master has an IP",
			args: args{
				pods: []corev1.Pod{
					newPodWithIP("master1", "", true),
					newPodWithIP("master2", "", true),
					newPodWithIP("master3", "", true),
					newPodWithIP("node1", "", false),
					newPodWithIP("node2", "10.0.2.8", false),
				},
				c:      k8s.WrapClient(fake.NewFakeClient()),
				es:     es,
				scheme: scheme.Scheme,
			},
			wantErr:         false,
			expectedContent: "",
		},
		{
			name: "Do not fail if there's no master at all",
			args: args{
				pods: []corev1.Pod{
					newPodWithIP("node1", "", false),
					newPodWithIP("node2", "10.0.2.8", false),
				},
				c:      k8s.WrapClient(fake.NewFakeClient()),
				es:     es,
				scheme: scheme.Scheme,
			},
			wantErr:         false,
			expectedContent: "",
		},
		{
			name: "One of the master doesn't have an IP",
			args: args{
				pods: []corev1.Pod{ //
					newPodWithIP("master1", "10.0.9.2", true),
					newPodWithIP("master2", "", true),
					newPodWithIP("master3", "10.0.3.3", true),
					newPodWithIP("node1", "10.0.9.3", false),
					newPodWithIP("node2", "10.0.2.8", false),
				},
				c:      k8s.WrapClient(fake.NewFakeClient()),
				es:     es,
				scheme: scheme.Scheme,
			},
			wantErr:         false,
			expectedContent: "10.0.9.2:9300\n10.0.3.3:9300",
		},
		{
			name: "All masters have IPs, some nodes don't",
			args: args{
				pods: []corev1.Pod{ //
					newPodWithIP("master1", "10.0.9.2", true),
					newPodWithIP("master2", "10.0.6.5", true),
					newPodWithIP("master3", "10.0.3.3", true),
					newPodWithIP("node1", "", false),
					newPodWithIP("node2", "10.0.2.8", false),
				},
				c:      k8s.WrapClient(fake.NewFakeClient()),
				es:     es,
				scheme: scheme.Scheme,
			},
			wantErr:         false,
			expectedContent: "10.0.9.2:9300\n10.0.6.5:9300\n10.0.3.3:9300",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := UpdateSeedHostsConfigMap(tt.args.c, tt.args.scheme, tt.args.es, tt.args.pods)
			if (err != nil) != tt.wantErr {
				t.Errorf("UpdateSeedHostsConfigMap() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Check the resulting confimap
			file := &corev1.ConfigMap{}
			if err := tt.args.c.Get(
				types.NamespacedName{
					Namespace: "ns1",
					Name:      name.UnicastHostsConfigMap(es.Name),
				}, file); err != nil {
				t.Errorf("Error while getting the seed hosts configmap: %v", err)
			}
			assert.Equal(t, len(file.Data), 1)
			assert.Equal(t, tt.expectedContent, file.Data[volume.UnicastHostsFile])
		})
	}
}
