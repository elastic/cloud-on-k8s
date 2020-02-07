// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.
package remotecluster

import (
	"reflect"
	"testing"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	esclient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_getCurrentRemoteClusters(t *testing.T) {
	type args struct {
		es esv1.Elasticsearch
	}
	tests := []struct {
		name    string
		args    args
		want    map[string]string
		wantErr bool
	}{
		{
			name: "Read from a nil annotation should be ok",
			args: args{es: esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "ns1",
					Namespace:   "es1",
					Annotations: map[string]string{},
				},
			}},
			want: nil,
		},
		{
			name: "Decode annotation into a list of remote cluster",
			args: args{es: esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "ns1",
					Namespace:   "es1",
					Annotations: map[string]string{"elasticsearch.k8s.elastic.co/remote-clusters": `[{"name":"ns2-cluster-2","configHash":"3795207740"}]`},
				},
			}},
			want: map[string]string{
				"ns2-cluster-2": "3795207740",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getCurrentRemoteClusters(tt.args.es)
			if (err != nil) != tt.wantErr {
				t.Errorf("getCurrentRemoteClusters() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getCurrentRemoteClusters() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_newRemoteClusterSetting(t *testing.T) {
	type args struct {
		name      string
		seedHosts []string
	}
	tests := []struct {
		name string
		args args
		want esclient.Settings
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := newRemoteClusterSetting(tt.args.name, tt.args.seedHosts); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("newRemoteClusterSetting() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_updateRemoteCluster(t *testing.T) {
	type args struct {
		esClient           esclient.Client
		persistentSettings esclient.Settings
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := updateRemoteCluster(tt.args.esClient, tt.args.persistentSettings); (err != nil) != tt.wantErr {
				t.Errorf("updateRemoteCluster() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
