// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package validation

import (
	"strings"
	"testing"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestResourcePolicies_Validate(t *testing.T) {
	tests := []struct {
		name            string
		autoscalingSpec string
		// NodeSet name -> roles
		nodeSets      map[string][]string
		volumeClaims  []string
		wantError     bool
		expectedError string
	}{
		{
			name:      "ML must be in a dedicated autoscaling policy",
			wantError: true,
			nodeSets:  map[string][]string{"nodeset-data-ml": {"data", "ml"}},
			autoscalingSpec: `
{
	 "policies" : [{
		  "name": "data_ml_policy",
		  "roles": [ "data", "ml" ],
		  "resources" : {
			"nodeCount" : { "min" : 1 , "max" : 2 },
			"cpu" : { "min" : 1 , "max" : 1 },
			"memory" : { "min" : "2Gi" , "max" : "2Gi" },
			"storage" : { "min" : "5Gi" , "max" : "10Gi" }
		  }
		}]
}
`,
			expectedError: "ML nodes must be in a dedicated autoscaling policy",
		},
		{
			name:      "ML is in a dedicated autoscaling policy",
			wantError: false,
			nodeSets:  map[string][]string{"nodeset-ml": {"ml"}},
			autoscalingSpec: `
{
	 "policies" : [{
		  "name": "ml",
		  "roles": [ "ml" ],
		  "resources" : {
			"nodeCount" : { "min" : 1 , "max" : 2 },
			"cpu" : { "min" : 1 , "max" : 1 },
			"memory" : { "min" : "2Gi" , "max" : "2Gi" },
			"storage" : { "min" : "5Gi" , "max" : "10Gi" }
		  }
		}]
}
`,
		},
		{
			name:      "Happy path",
			wantError: false,
			nodeSets:  map[string][]string{"nodeset-data-1": {"data"}, "nodeset-data-2": {"data"}, "nodeset-ml": {"ml"}},
			autoscalingSpec: `
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
}
`,
		},
		{
			name:          "Autoscaling policy with no NodeSet",
			wantError:     true,
			expectedError: "Invalid value: []string{\"ml\"}: roles must be used in at least one nodeSet",
			nodeSets:      map[string][]string{"nodeset-data-1": {"data"}, "nodeset-data-2": {"data"}},
			autoscalingSpec: `
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
}
`,
		},
		{
			name:          "nodeSet with no roles",
			wantError:     true,
			expectedError: "cannot parse nodeSet configuration: node.roles must be set",
			nodeSets:      map[string][]string{"nodeset-data-1": nil, "nodeset-data-2": {"data"}},
			autoscalingSpec: `
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
}
`,
		},
		{
			name:          "Min memory is 2G",
			wantError:     true,
			expectedError: "min quantity must be greater than 2G",
			autoscalingSpec: `
{
	 "policies" : [{
		  "name": "data_policy",
		  "roles": [ "data" ],
		  "resources" : {
			"nodeCount" : { "min" : 1 , "max" : 2 },
			"cpu" : { "min" : 1 , "max" : 1 },
			"memory" : { "min" : "1Gi" , "max" : "2Gi" },
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
}
`,
		},
		{
			name:          "Policy name is duplicated",
			wantError:     true,
			expectedError: "[1].name: Invalid value: \"my_policy\": policy is duplicated",
			autoscalingSpec: `
{
	 "policies" : [{
		  "name": "my_policy",
		  "roles": [ "data" ],
		  "resources" : {
			"nodeCount" : { "min" : 1 , "max" : 2 },
			"cpu" : { "min" : 1 , "max" : 1 },
			"memory" : { "min" : "2Gi" , "max" : "2Gi" },
			"storage" : { "min" : "5Gi" , "max" : "10Gi" }
		  }
		},
		{
		  "name": "my_policy",
		  "roles": [ "ml" ],
		  "resources" : {
			"nodeCount" : { "min" : 1 , "max" : 2 },
			"cpu" : { "min" : 1 , "max" : 1 },
			"memory" : { "min" : "2Gi" , "max" : "2Gi" },
			"storage" : { "min" : "5Gi" , "max" : "10Gi" }
		  }
		}]
}
`,
		},
		{
			name:          "Duplicated roles sets",
			nodeSets:      map[string][]string{"nodeset-data-2": {"data_hot", "data_content"}},
			wantError:     true,
			expectedError: "autoscaling-spec\"[1].name: Invalid value: \"data_content,data_hot\": roles set is duplicated",
			autoscalingSpec: `
{
	 "policies" : [{
		  "name": "my_policy",
		  "roles": [ "data_hot", "data_content" ],
		  "resources" : {
			  "nodeCount" : { "min" : 1 , "max" : 2 },
			  "cpu" : { "min" : 1 , "max" : 1 },
			  "memory" : { "min" : "2Gi" , "max" : "2Gi" },
			  "storage" : { "min" : "5Gi" , "max" : "10Gi" }
          }
		},
		{
		  "name": "my_policy2",
		  "roles": [ "data_content", "data_hot" ],
		  "resources" : {
			  "nodeCount" : { "min" : 1 , "max" : 2 },
			  "cpu" : { "min" : 1 , "max" : 1 },
			  "memory" : { "min" : "2Gi" , "max" : "2Gi" },
			  "storage" : { "min" : "5Gi" , "max" : "10Gi" }
		  }
		}]
}
`,
		},
		{
			name:          "No name",
			wantError:     true,
			expectedError: "name: Required value: name is mandatory",
			autoscalingSpec: `
{
	 "policies" : [{
	  "roles": [ "data", "ml" ],
      "resources" : {
		  "nodeCount" : { "min" : 1 , "max" : 2 },
		  "cpu" : { "min" : 1 , "max" : 1 },
		  "memory" : { "min" : "2Gi" , "max" : "2Gi" },
		  "storage" : { "min" : "5Gi" , "max" : "10Gi" }
      }
	}]
}
`,
		},
		{
			name:          "No roles",
			wantError:     true,
			expectedError: "roles: Required value: roles field is mandatory",
			autoscalingSpec: `
{
	 "policies" : [{
     "name": "my_policy",
      "resources" : {
		  "nodeCount" : { "min" : 1 , "max" : 2 },
		  "cpu" : { "min" : 1 , "max" : 1 },
		  "memory" : { "min" : "2Gi" , "max" : "2Gi" },
		  "storage" : { "min" : "5Gi" , "max" : "10Gi" }
      }
      }]
}
`,
		},
		{
			name:          "No count",
			nodeSets:      map[string][]string{"nodeset-data-1": {"ml"}},
			wantError:     true,
			expectedError: "resources.nodeCount.max: Invalid value: 0: max count must be greater than 0",
			autoscalingSpec: `
{
	 "policies" : [{
  "name": "my_policy",
  "roles": [ "ml" ],
  "resources" : {
	  "cpu" : { "min" : 1 , "max" : 1 },
	  "memory" : { "min" : "2Gi" , "max" : "2Gi" },
	  "storage" : { "min" : "5Gi" , "max" : "10Gi" }
  }
}]
}
`,
		},
		{
			name:          "Min. count should be equal or greater than 0",
			nodeSets:      map[string][]string{"nodeset-data-1": {"ml"}},
			wantError:     true,
			expectedError: "resources.nodeCount.min: Invalid value: -1: min count must be equal or greater than 0",
			autoscalingSpec: `
{
	"policies": [{
		"name": "my_policy",
		"roles": ["ml"],
		"resources": {
			"nodeCount": {
				"min": -1,
				"max": 2
			},
			"cpu": {
				"min": 1,
				"max": 1
			},
			"memory": {
				"min": "2Gi",
				"max": "2Gi"
			},
			"storage": {
				"min": "5Gi",
				"max": "10Gi"
			}
		}
	}]
}
`,
		},
		{
			name:      "Min. count is 0 max count must be greater than 0",
			nodeSets:  map[string][]string{"nodeset-data-1": {"ml"}},
			wantError: true,
			autoscalingSpec: `
{
	"policies": [{
		"name": "my_policy",
		"roles": ["ml"],
		"resources": {
			"nodeCount": {
				"min": 0,
				"max": 0
			},
			"cpu": {
				"min": 1,
				"max": 1
			},
			"memory": {
				"min": "2Gi",
				"max": "2Gi"
			},
			"storage": {
				"min": "5Gi",
				"max": "10Gi"
			}
		}
	}]
}
`,
		},
		{
			name:      "Min. count and max count are equal",
			nodeSets:  map[string][]string{"nodeset-data-1": {"ml"}},
			wantError: false,
			autoscalingSpec: `
{
	"policies": [{
		"name": "my_policy",
        "roles": [ "ml" ],
		"resources": {
			"nodeCount": {
				"min": 2,
				"max": 2
			},
			"cpu": {
				"min": 1,
				"max": 1
			},
			"memory": {
				"min": "2Gi",
				"max": "2Gi"
			},
			"storage": {
				"min": "5Gi",
				"max": "10Gi"
			}
		}
	}]
}
`,
		},
		{
			name:          "Min. count is greater than max",
			nodeSets:      map[string][]string{"nodeset-data-1": {"ml"}},
			wantError:     true,
			expectedError: "resources.nodeCount.max: Invalid value: 4: max node count must be an integer greater or equal than the min node count",
			autoscalingSpec: `
{
	"policies": [{
		"name": "my_policy",
        "roles": [ "ml" ],
		"resources": {
			"nodeCount": {
				"min": 5,
				"max": 4
			},
			"cpu": {
				"min": 1,
				"max": 1
			},
			"memory": {
				"min": "2Gi",
				"max": "2Gi"
			},
			"storage": {
				"min": "5Gi",
				"max": "10Gi"
			}
		}
	}]
}
`,
		},
		{
			name:          "Min. CPU is greater than max",
			nodeSets:      map[string][]string{"nodeset-data-1": {"ml"}},
			wantError:     true,
			expectedError: "cpu: Invalid value: \"50m\": max quantity must be greater or equal than min quantity",
			autoscalingSpec: `
{
	"policies": [{
		"name": "my_policy",
        "roles": [ "ml" ],
		"resources": {
			"nodeCount": {
				"min": -1,
				"max": 2
			},
			"cpu": {
				"min": "100m",
				"max": "50m"
			},
			"memory": {
				"min": "2Gi",
				"max": "2Gi"
			},
			"storage": {
				"min": "5Gi",
				"max": "10Gi"
			}
		}
	}]
}
`,
		},
		{
			name:         "Default volume claim",
			wantError:    false,
			nodeSets:     map[string][]string{"ml": {"ml"}},
			volumeClaims: []string{"elasticsearch-data"},
			autoscalingSpec: `
{
	 "policies" : [{
		  "name": "ml_policy",
		  "roles": [ "ml" ],
		  "resources" : {
			"nodeCount" : { "min" : 1 , "max" : 2 },
			"cpu" : { "min" : 1 , "max" : 1 },
			"memory" : { "min" : "2Gi" , "max" : "2Gi" },
			"storage" : { "min" : "5Gi" , "max" : "10Gi" }
		  }
		}]
}
`,
		},
		{
			name:         "Not the default volume claim",
			wantError:    false,
			nodeSets:     map[string][]string{"ml": {"ml"}},
			volumeClaims: []string{"volume1"},
			autoscalingSpec: `
{
	 "policies" : [{
		  "name": "ml_policy",
		  "roles": [ "ml" ],
		  "resources" : {
			"nodeCount" : { "min" : 1 , "max" : 2 },
			"cpu" : { "min" : 1 , "max" : 1 },
			"memory" : { "min" : "2Gi" , "max" : "2Gi" },
			"storage" : { "min" : "5Gi" , "max" : "10Gi" }
		  }
		}]
}
`,
		},
		{
			name:         "More than one volume claim",
			wantError:    true,
			nodeSets:     map[string][]string{"ml": {"ml"}},
			volumeClaims: []string{"volume1", "volume2"},
			autoscalingSpec: `
{
	 "policies" : [{
		  "name": "ml_policy",
		  "roles": [ "ml" ],
		  "resources" : {
			"nodeCount" : { "min" : 1 , "max" : 2 },
			"cpu" : { "min" : 1 , "max" : 1 },
			"memory" : { "min" : "2Gi" , "max" : "2Gi" },
			"storage" : { "min" : "5Gi" , "max" : "10Gi" }
		  }
		}]
}
`,
			expectedError: "autoscaling supports only one volume claim",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			es := esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						esv1.ElasticsearchAutoscalingSpecAnnotationName: tt.autoscalingSpec,
					},
				},
				Spec: esv1.ElasticsearchSpec{
					Version: "7.11.0",
				},
			}
			for nodeSetName, roles := range tt.nodeSets {
				cfg := commonv1.NewConfig(map[string]interface{}{})
				if roles != nil {
					cfg = commonv1.NewConfig(map[string]interface{}{"node.roles": roles})
				}
				volumeClaimTemplates := volumeClaimTemplates(tt.volumeClaims)
				nodeSet := esv1.NodeSet{
					Name:                 nodeSetName,
					Config:               &cfg,
					VolumeClaimTemplates: volumeClaimTemplates,
				}
				es.Spec.NodeSets = append(es.Spec.NodeSets, nodeSet)
			}
			got := validAutoscalingConfiguration(es)
			assert.Equal(t, tt.wantError, got != nil)
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

func volumeClaimTemplates(volumeClaims []string) []corev1.PersistentVolumeClaim {
	volumeClaimTemplates := make([]corev1.PersistentVolumeClaim, len(volumeClaims))
	for i := range volumeClaims {
		volumeClaimTemplates[i] = corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name: volumeClaims[i],
			},
		}
	}
	return volumeClaimTemplates
}
