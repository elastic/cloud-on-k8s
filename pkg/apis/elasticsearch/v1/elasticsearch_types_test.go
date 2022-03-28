// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1

import (
	"fmt"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/utils/pointer"
	"github.com/elastic/cloud-on-k8s/pkg/utils/set"
)

func TestElasticsearchHealth_Less(t *testing.T) {
	tests := []struct {
		inputs []ElasticsearchHealth
		sorted bool
	}{
		{
			inputs: []ElasticsearchHealth{
				"",
				ElasticsearchYellowHealth,
				"",
			},
			sorted: true,
		},
		{
			inputs: []ElasticsearchHealth{
				ElasticsearchUnknownHealth,
				ElasticsearchYellowHealth,
				ElasticsearchUnknownHealth,
			},
			sorted: true,
		},
		{
			inputs: []ElasticsearchHealth{
				ElasticsearchRedHealth,
				ElasticsearchYellowHealth,
			},
			sorted: true,
		},
		{
			inputs: []ElasticsearchHealth{
				ElasticsearchRedHealth,
				ElasticsearchRedHealth,
			},
			sorted: true,
		},
		{
			inputs: []ElasticsearchHealth{
				ElasticsearchRedHealth,
				ElasticsearchGreenHealth,
			},
			sorted: true,
		},
		{
			inputs: []ElasticsearchHealth{
				ElasticsearchRedHealth,
				ElasticsearchYellowHealth,
				ElasticsearchGreenHealth,
			},
			sorted: true,
		},
		{
			inputs: []ElasticsearchHealth{
				ElasticsearchYellowHealth,
				ElasticsearchGreenHealth,
			},
			sorted: true,
		},
		{
			inputs: []ElasticsearchHealth{
				ElasticsearchGreenHealth,
				ElasticsearchYellowHealth,
			},
			sorted: false,
		},
	}

	for _, tt := range tests {
		assert.Equal(t, sort.SliceIsSorted(tt.inputs, func(i, j int) bool {
			return tt.inputs[i].Less(tt.inputs[j])
		}), tt.sorted, fmt.Sprintf("%v", tt.inputs))
	}
}

func TestElasticsearchCluster_IsMarkedForDeletion(t *testing.T) {
	zeroTime := metav1.NewTime(time.Time{})
	currentTime := metav1.NewTime(time.Now())
	tests := []struct {
		name              string
		deletionTimestamp *metav1.Time
		want              bool
	}{
		{
			name:              "deletion timestamp nil",
			deletionTimestamp: nil,
			want:              false,
		},
		{
			name:              "deletion timestamp set to its zero value",
			deletionTimestamp: &zeroTime,
			want:              false,
		},
		{
			name:              "deletion timestamp set to any non-zero value",
			deletionTimestamp: &currentTime,
			want:              true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					DeletionTimestamp: tt.deletionTimestamp,
				},
			}
			require.Equal(t, tt.want, e.IsMarkedForDeletion())
		})
	}
}
func Test_GetMaxSurgeOrDefault(t *testing.T) {
	tests := []struct {
		name     string
		fromSpec *int32
		want     *int32
	}{
		{
			name:     "negative in spec results in unbounded",
			fromSpec: pointer.Int32(-1),
			want:     nil,
		},
		{
			name:     "nil in spec results in default, generic",
			fromSpec: nil,
			want:     DefaultChangeBudget.MaxSurge,
		},
		{
			name:     "nil in spec results in default, currently nil",
			fromSpec: nil,
			want:     nil,
		},
		{
			name:     "0 in spec results in 0",
			fromSpec: pointer.Int32(0),
			want:     pointer.Int32(0),
		},
		{
			name:     "1 in spec results in 1",
			fromSpec: pointer.Int32(1),
			want:     pointer.Int32(1),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ChangeBudget{MaxSurge: tt.fromSpec}.GetMaxSurgeOrDefault()
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetMaxSurgeOrDefault() want = %v, got = %v", tt.want, got)
			}
		})
	}
}

func Test_GetMaxUnavailableOrDefault(t *testing.T) {
	tests := []struct {
		name     string
		fromSpec *int32
		want     *int32
	}{
		{
			name:     "negative in spec results in unbounded",
			fromSpec: pointer.Int32(-1),
			want:     nil,
		},
		{
			name:     "nil in spec results in default, generic",
			fromSpec: nil,
			want:     DefaultChangeBudget.MaxUnavailable,
		},
		{
			name:     "nil in spec results in default, currently 1",
			fromSpec: nil,
			want:     pointer.Int32(1),
		},
		{
			name:     "0 in spec results in 0",
			fromSpec: pointer.Int32(0),
			want:     pointer.Int32(0),
		},
		{
			name:     "1 in spec results in 1",
			fromSpec: pointer.Int32(1),
			want:     pointer.Int32(1),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ChangeBudget{MaxUnavailable: tt.fromSpec}.GetMaxUnavailableOrDefault()
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetMaxUnavailableOrDefault() want = %v, got = %v", tt.want, got)
			}
		})
	}
}

func TestElasticsearch_SuspendedPodNames(t *testing.T) {
	tests := []struct {
		name       string
		ObjectMeta metav1.ObjectMeta
		want       set.StringSet
	}{
		{
			name:       "no annotation",
			ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{}},
			want:       nil,
		},
		{
			name: "single value",
			ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
				SuspendAnnotation: "a",
			}},
			want: set.Make("a"),
		},
		{
			name: "multi value",
			ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
				SuspendAnnotation: "a,b,c",
			}},
			want: set.Make("a", "b", "c"),
		},
		{
			name: "multi value with whitespace",
			ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
				SuspendAnnotation: "a , b , c",
			}},
			want: set.Make("a", "b", "c"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			es := Elasticsearch{
				ObjectMeta: tt.ObjectMeta,
			}
			if got := es.SuspendedPodNames(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("SuspendedPodNames() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestElasticsearch_DisabledPredicates(t *testing.T) {
	tests := []struct {
		name string
		es   Elasticsearch
		want set.StringSet
	}{
		{
			name: "no annotations",
			es: Elasticsearch{ObjectMeta: metav1.ObjectMeta{
				Annotations: nil,
			}},
			want: nil,
		},
		{
			name: "no annotation",
			es: Elasticsearch{ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{},
			}},
			want: nil,
		},
		{
			name: "1 disabled predicate",
			es: Elasticsearch{ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					DisableUpgradePredicatesAnnotation: "foo",
				},
			}},
			want: set.Make("foo"),
		},
		{
			name: "2 disabled predicates",
			es: Elasticsearch{ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					DisableUpgradePredicatesAnnotation: "foo,bar",
				},
			}},
			want: set.Make("foo", "bar"),
		},
		{
			name: "all predicates disabled",
			es: Elasticsearch{ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					DisableUpgradePredicatesAnnotation: "*",
				},
			}},
			want: set.Make("*"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.es.DisabledPredicates(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Elasticsearch.DisabledPredicates() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Test_AssociationConfs tests that if the association configuration map in an associated object is cleared, then
// AssociationConf() is rebuilt from the annotation.
func Test_AssociationConfs(t *testing.T) {
	// simple es without associations
	es := &Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "es",
			Namespace: "default",
		},
	}
	assert.Equal(t, 0, len(es.GetAssociations()))
	assert.Equal(t, 0, len(es.AssocConfs))

	// es with associations
	metricsEsRef := commonv1.ObjectSelector{
		Name:      "metrics",
		Namespace: "default",
	}
	esMon := &Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "esmon",
			Namespace: "default",
			Annotations: map[string]string{
				"association.k8s.elastic.co/es-conf-864518565":  `{"authSecretName":"es-default-metrics-beat-es-mon-user","authSecretKey":"default-es-default-esmon-beat-es-mon-user","caCertProvided":true,"caSecretName":"es-es-monitoring-default-metrics-ca","url":"https://metrics-es-http.default.svc:9200","version":"8.0.0"}`,
				"association.k8s.elastic.co/es-conf-1654136115": `{"authSecretName":"es-default-logs-beat-es-mon-user","authSecretKey":"default-es-default-esmon-beat-es-mon-user","caCertProvided":true,"caSecretName":"es-es-monitoring-default-logs-ca","url":"https://logs-es-http.default.svc:9200","version":"8.0.0"}`,
			},
		},
		Spec: ElasticsearchSpec{
			Monitoring: Monitoring{
				Metrics: MetricsMonitoring{
					ElasticsearchRefs: []commonv1.ObjectSelector{metricsEsRef},
				},
				Logs: LogsMonitoring{
					ElasticsearchRefs: []commonv1.ObjectSelector{{
						Name:      "logs",
						Namespace: "default"},
					},
				},
			},
		},
	}
	assert.Equal(t, 2, len(esMon.GetAssociations()))

	// map should be initially empty
	assert.Equal(t, 0, len(esMon.AssocConfs))

	// get and set assoc conf
	for _, assoc := range esMon.GetAssociations() {
		assocConf, err := assoc.AssociationConf()
		assert.NotNil(t, assocConf)
		assert.NoError(t, err)
	}
	// map should have been populated by the call to AssociationConf()
	assert.Equal(t, 2, len(esMon.AssocConfs))

	// simulate the case where the assocConfs map is reset, which can happen if the resource is updated
	esMon.AssocConfs = nil
	assert.Equal(t, 0, len(esMon.AssocConfs))

	// get and set assoc conf
	for _, assoc := range esMon.GetAssociations() {
		assocConf, err := assoc.AssociationConf()
		assert.NotNil(t, assocConf)
		assert.NoError(t, err)
	}
	// checks that all map entries are set again
	assert.Equal(t, 2, len(esMon.AssocConfs))

	// delete just one entry in the map
	delete(esMon.AssocConfs, metricsEsRef.NamespacedName())
	assert.Equal(t, 1, len(esMon.AssocConfs))

	// checks that the missing entry is set again
	for _, assoc := range esMon.GetAssociations() {
		assocConf, err := assoc.AssociationConf()
		assert.NotNil(t, assocConf)
		assert.NoError(t, err)
	}
	assert.Equal(t, 2, len(esMon.AssocConfs))
}
