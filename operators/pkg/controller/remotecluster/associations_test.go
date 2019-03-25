// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package remotecluster

import (
	"reflect"
	"testing"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

func newTrustRelationship(namespace, name, caCert string, subjectName []string) v1alpha1.TrustRelationship {
	return v1alpha1.TrustRelationship{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: v1alpha1.TrustRelationshipSpec{
			TrustRestrictions: v1alpha1.TrustRestrictions{
				Trust: v1alpha1.Trust{
					SubjectName: subjectName,
				},
			},
			CaCert: caCert,
		},
	}
}

func Test_newAssociatedCluster(t *testing.T) {
	type args struct {
		c        k8s.Client
		selector v1alpha1.ObjectSelector
	}
	tests := []struct {
		name    string
		args    args
		want    associatedCluster
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := newAssociatedCluster(tt.args.c, tt.args.selector)
			if (err != nil) != tt.wantErr {
				t.Errorf("newAssociatedCluster() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("newAssociatedCluster() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_ensureTrustRelationshipIsDeleted(t *testing.T) {
	trustRelationShip1 := newTrustRelationship("ns1", "trustrelationship1", ca1, []string{})
	type args struct {
		c       k8s.Client
		name    string
		owner   *v1alpha1.RemoteCluster
		cluster v1alpha1.ObjectSelector
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "Delete a trust relationship that does exist",
			args: args{
				c:     newFakeClient(t, []runtime.Object{&trustRelationShip1}),
				name:  "trustrelationship1",
				owner: newRemoteInCluster("remotecluster-sample", "ns1", "es1", "ns2", "es2"),
				cluster: v1alpha1.ObjectSelector{
					Namespace: "ns1",
					Name:      "es1",
				},
			},
		},
		{
			name: "Delete a trust relationship that does not exist, no error expected",
			args: args{
				c:     newFakeClient(t, []runtime.Object{}),
				name:  "trustrelationship1",
				owner: newRemoteInCluster("remotecluster-sample", "ns1", "es1", "ns2", "es2"),
				cluster: v1alpha1.ObjectSelector{
					Namespace: "ns1",
					Name:      "es1",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ensureTrustRelationshipIsDeleted(tt.args.c, tt.args.name, tt.args.owner, tt.args.cluster); (err != nil) != tt.wantErr {
				t.Errorf("ensureTrustRelationshipIsDeleted() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_getCA(t *testing.T) {
	type args struct {
		c  k8s.Client
		es types.NamespacedName
	}
	ca1bytes := []byte(ca1)
	tests := []struct {
		name    string
		args    args
		want    *[]byte
		wantErr bool
	}{
		{
			name: "CA cert does not exist",
			args: args{
				c: newFakeClient(t, []runtime.Object{}),
				es: types.NamespacedName{
					Namespace: "default",
					Name:      "foo",
				},
			},
			want: nil,
		}, {
			name: "CA cert does exist",
			args: args{
				c: newFakeClient(t, []runtime.Object{
					newCASecret("default", "trust-one-es-ca", ca1),
				}),
				es: types.NamespacedName{
					Namespace: "default",
					Name:      "trust-one-es",
				},
			},
			want: &ca1bytes,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getCA(tt.args.c, tt.args.es)
			if (err != nil) != tt.wantErr {
				t.Errorf("getCA() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getCA() = %v, want %v", got, tt.want)
			}
		})
	}
}
