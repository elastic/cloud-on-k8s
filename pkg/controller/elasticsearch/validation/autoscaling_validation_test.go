// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package validation

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
)

const (
	autoscalingSpec = `
{
	 "policies" : [{
		  "name": "data_policy",
		  "roles": [ "data" ],
		  "resources" : {
			"nodeCount" : { "min" : 1 , "max" : 2 },
			"cpu" : { "min" : 1 , "max" : 1 },
			"memory" : { "min" : "2Gi" , "max" : "2Gi" },
			"storage" : { "min" : "5Gi" , "max" : "10Gi" }
		  }
		},
		{
		  "name": "ml_policy",
		  "roles": [ "ml" ],
		  "resources" : {
			"nodeCount" : { "min" : 1 , "max" : 2 },
			"cpu" : { "min" : 1 , "max" : 1 },
			"memory" : { "min" : "2Gi" , "max" : "2Gi" },
			"storage" : { "min" : "5Gi" , "max" : "10Gi" }
		  }
		}]
}`
)

func TestResourcePolicies_Validate(t *testing.T) {
	tests := []struct {
		name          string
		version       string
		wantError     bool
		expectedError string
	}{
		{
			name:          "Autoscaling annotation not supported",
			wantError:     true,
			version:       "7.11.0",
			expectedError: "metadata.annotations.elasticsearch.alpha.elastic.co/autoscaling-spec: Invalid value: \"elasticsearch.alpha.elastic.co/autoscaling-spec\": autoscaling annotation is no longer supported",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			es := esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						esv1.ElasticsearchAutoscalingSpecAnnotationName: autoscalingSpec,
					},
				},
				Spec: esv1.ElasticsearchSpec{
					Version: tt.version,
				},
			}
			for nodeSetName, roles := range map[string][]string{"nodeset-data-1": {"data"}, "nodeset-data-2": {"data"}, "nodeset-ml": {"ml"}} {
				cfg := commonv1.NewConfig(map[string]interface{}{})
				if roles != nil {
					cfg = commonv1.NewConfig(map[string]interface{}{"node.roles": roles})
				}
				nodeSet := esv1.NodeSet{
					Name:                 nodeSetName,
					Config:               &cfg,
					VolumeClaimTemplates: []corev1.PersistentVolumeClaim{{ObjectMeta: metav1.ObjectMeta{Name: "elasticsearch-data"}}},
				}
				es.Spec.NodeSets = append(es.Spec.NodeSets, nodeSet)
			}
			got := validAutoscalingConfiguration(es)
			require.Equal(t, tt.wantError, len(got) > 0)
			found := false
			for _, gotErr := range got {
				if strings.Contains(gotErr.Error(), tt.expectedError) {
					found = true
					break
				}
			}

			if tt.wantError && !found {
				t.Errorf("AutoscalingSpecs.Validate() = %v, want string \"%v\"", got, tt.expectedError)
			}
		})
	}
}
