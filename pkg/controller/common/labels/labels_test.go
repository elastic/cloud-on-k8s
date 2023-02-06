// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package labels

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	agent "github.com/elastic/cloud-on-k8s/v2/pkg/apis/agent/v1alpha1"
	apm "github.com/elastic/cloud-on-k8s/v2/pkg/apis/apm/v1"
	beat "github.com/elastic/cloud-on-k8s/v2/pkg/apis/beat/v1beta1"
	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	es "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	enterprisesearch "github.com/elastic/cloud-on-k8s/v2/pkg/apis/enterprisesearch/v1"
	kibana "github.com/elastic/cloud-on-k8s/v2/pkg/apis/kibana/v1"
	maps "github.com/elastic/cloud-on-k8s/v2/pkg/apis/maps/v1alpha1"
)

const testLabel TrueFalseLabel = "foo"

func TestTrueFalseLabel_Set(t *testing.T) {
	type args struct {
		value  bool
		labels map[string]string
	}
	tests := []struct {
		name string
		l    TrueFalseLabel
		args args
		want map[string]string
	}{
		{
			name: "true",
			l:    testLabel,
			args: args{
				value:  true,
				labels: map[string]string{},
			},
			want: map[string]string{"foo": "true"},
		},
		{
			name: "talse",
			l:    testLabel,
			args: args{
				value:  false,
				labels: map[string]string{},
			},
			want: map[string]string{"foo": "false"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.l.Set(tt.args.value, tt.args.labels)
			assert.Equal(t, tt.want, tt.args.labels)
		})
	}
}

func TestTrueFalseLabel_HasValue(t *testing.T) {
	type args struct {
		value  bool
		labels map[string]string
	}
	tests := []struct {
		name string
		l    TrueFalseLabel
		args args
		want bool
	}{
		{
			name: "unset, true",
			l:    testLabel,
			args: args{
				value:  true,
				labels: map[string]string{},
			},
			want: false,
		},
		{
			name: "unset, false",
			l:    testLabel,
			args: args{
				value:  false,
				labels: map[string]string{},
			},
			want: false,
		},
		{
			name: "set unexpected, true",
			l:    testLabel,
			args: args{
				value:  true,
				labels: map[string]string{"foo": "unexpected"},
			},
			want: false,
		},
		{
			name: "set unexpected, false",
			l:    testLabel,
			args: args{
				value:  false,
				labels: map[string]string{"foo": "unexpected"},
			},
			want: false,
		},
		{
			name: "set true, true",
			l:    testLabel,
			args: args{
				value:  true,
				labels: map[string]string{"foo": "true"},
			},
			want: true,
		},
		{
			name: "set true, false",
			l:    testLabel,
			args: args{
				value:  true,
				labels: map[string]string{"foo": "false"},
			},
			want: false,
		},
		{
			name: "set false, false",
			l:    testLabel,
			args: args{
				value:  false,
				labels: map[string]string{"foo": "false"},
			},
			want: true,
		},
		{
			name: "set false, true",
			l:    testLabel,
			args: args{
				value:  false,
				labels: map[string]string{"foo": "true"},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.l.HasValue(tt.args.value, tt.args.labels); got != tt.want {
				t.Errorf("TrueFalseLabel.HasValue() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTrueFalseLabel_AsMap(t *testing.T) {
	type args struct {
		value bool
	}
	tests := []struct {
		name string
		l    TrueFalseLabel
		args args
		want map[string]string
	}{
		{
			name: "true",
			l:    testLabel,
			args: args{value: true},
			want: map[string]string{"foo": "true"},
		},
		{
			name: "false",
			l:    testLabel,
			args: args{value: false},
			want: map[string]string{"foo": "false"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.l.AsMap(tt.args.value)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGetIdentityLabels(t *testing.T) {
	tests := []struct {
		name       string
		obj        commonv1.HasIdentityLabels
		wantLabels map[string]string
	}{
		{
			name: "Agent returns the correct labels",
			obj: &agent.Agent{
				ObjectMeta: v1.ObjectMeta{
					Name: "test-agent",
				},
			},
			wantLabels: map[string]string{
				commonv1.TypeLabelName:      "agent",
				"agent.k8s.elastic.co/name": "test-agent",
			},
		},
		{
			name: "ApmServer returns the correct labels",
			obj: &apm.ApmServer{
				ObjectMeta: v1.ObjectMeta{
					Name: "test-apmserver",
				},
			},
			wantLabels: map[string]string{
				commonv1.TypeLabelName:    "apm-server",
				"apm.k8s.elastic.co/name": "test-apmserver",
			},
		},
		{
			name: "Beat returns the correct labels",
			obj: &beat.Beat{
				ObjectMeta: v1.ObjectMeta{
					Name: "test-beat",
				},
			},
			wantLabels: map[string]string{
				commonv1.TypeLabelName:     "beat",
				"beat.k8s.elastic.co/name": "test-beat",
			},
		},
		{
			name: "Elasticsearch returns the correct labels",
			obj: &es.Elasticsearch{
				ObjectMeta: v1.ObjectMeta{
					Name: "test-elasticsearch",
				},
			},
			wantLabels: map[string]string{
				commonv1.TypeLabelName:                      "elasticsearch",
				"elasticsearch.k8s.elastic.co/cluster-name": "test-elasticsearch",
			},
		},
		{
			name: "EnterpriseSearch returns the correct labels",
			obj: &enterprisesearch.EnterpriseSearch{
				ObjectMeta: v1.ObjectMeta{
					Name: "test-es",
				},
			},
			wantLabels: map[string]string{
				commonv1.TypeLabelName:                 "enterprise-search",
				"enterprisesearch.k8s.elastic.co/name": "test-es",
			},
		},
		{
			name: "Kibana returns the correct labels",
			obj: &kibana.Kibana{
				ObjectMeta: v1.ObjectMeta{
					Name: "test-kb",
				},
			},
			wantLabels: map[string]string{
				commonv1.TypeLabelName:       "kibana",
				"kibana.k8s.elastic.co/name": "test-kb",
			},
		},
		{
			name: "Maps returns the correct labels",
			obj: &maps.ElasticMapsServer{
				ObjectMeta: v1.ObjectMeta{
					Name: "test-ems",
				},
			},
			wantLabels: map[string]string{
				commonv1.TypeLabelName:     "maps",
				"maps.k8s.elastic.co/name": "test-ems",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if gotLabels := tt.obj.GetIdentityLabels(); !reflect.DeepEqual(gotLabels, tt.wantLabels) {
				t.Errorf("New() = %v, want %v", gotLabels, tt.wantLabels)
			}
		})
	}
}
