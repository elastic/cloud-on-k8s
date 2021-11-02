// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package k8s

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestOverrideControllerReference(t *testing.T) {
	ownerRefFixture := func(name string, controller bool) metav1.OwnerReference {
		return metav1.OwnerReference{
			APIVersion: "v1",
			Kind:       "some",
			Name:       name,
			UID:        "uid",
			Controller: &controller,
		}
	}
	type args struct {
		obj      metav1.Object
		newOwner metav1.OwnerReference
	}
	tests := []struct {
		name      string
		args      args
		assertion func(object metav1.Object)
	}{
		{
			name: "no existing controller",
			args: args{
				obj:      &corev1.Secret{},
				newOwner: ownerRefFixture("obj1", true),
			},
			assertion: func(object metav1.Object) {
				require.Equal(t, object.GetOwnerReferences(), []metav1.OwnerReference{ownerRefFixture("obj1", true)})
			},
		},
		{
			name: "replace existing controller",
			args: args{
				obj: &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						OwnerReferences: []metav1.OwnerReference{
							ownerRefFixture("obj1", true),
						},
					},
				},
				newOwner: ownerRefFixture("obj2", true),
			},
			assertion: func(object metav1.Object) {
				require.Equal(t, object.GetOwnerReferences(), []metav1.OwnerReference{
					ownerRefFixture("obj2", true)})
			},
		},
		{
			name: "replace existing controller preserving existing references",
			args: args{
				obj: &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						OwnerReferences: []metav1.OwnerReference{
							ownerRefFixture("other", false),
							ownerRefFixture("obj1", true),
						},
					},
				},
				newOwner: ownerRefFixture("obj2", true),
			},
			assertion: func(object metav1.Object) {
				require.Equal(t, object.GetOwnerReferences(), []metav1.OwnerReference{
					ownerRefFixture("other", false),
					ownerRefFixture("obj2", true)})
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			OverrideControllerReference(tt.args.obj, tt.args.newOwner)
			tt.assertion(tt.args.obj)
		})
	}
}

func sampleOwner() *corev1.Secret {
	// we use a secret here but it could be any Elasticsearch | Kibana | ApmServer | etc.
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "owner-name", UID: "owner-id"},
		TypeMeta:   metav1.TypeMeta{Kind: "Secret"},
	}
}

func addOwner(secret *corev1.Secret, name string, uid types.UID) *corev1.Secret {
	secret = secret.DeepCopy()
	secret.OwnerReferences = append(secret.OwnerReferences, metav1.OwnerReference{Name: name, UID: uid})
	return secret
}

func Test_hasOwner(t *testing.T) {
	owner := sampleOwner()
	type args struct {
		resource metav1.Object
		owner    metav1.Object
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "owner is referenced (same name and uid)",
			args: args{
				resource: addOwner(&corev1.Secret{}, owner.Name, owner.UID),
				owner:    owner,
			},
			want: true,
		},
		{
			name: "owner referenced among other owner references",
			args: args{
				resource: addOwner(addOwner(&corev1.Secret{}, "another-name", types.UID("another-id")), owner.Name, owner.UID),
				owner:    owner,
			},
			want: true,
		},
		{
			name: "owner not referenced",
			args: args{
				resource: addOwner(addOwner(&corev1.Secret{}, "another-name", types.UID("another-id")), "yet-another-name", "yet-another-uid"),
				owner:    owner,
			},
			want: false,
		},
		{
			name: "no owner ref",
			args: args{
				resource: &corev1.Secret{},
				owner:    owner,
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HasOwner(tt.args.resource, tt.args.owner); got != tt.want {
				t.Errorf("hasOwner() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_removeOwner(t *testing.T) {
	type args struct {
		resource metav1.Object
		owner    metav1.Object
	}
	tests := []struct {
		name         string
		args         args
		wantResource *corev1.Secret
	}{
		{
			name: "no owner: no-op",
			args: args{
				resource: &corev1.Secret{},
				owner:    sampleOwner(),
			},
			wantResource: &corev1.Secret{},
		},
		{
			name: "different owner: no-op",
			args: args{
				resource: addOwner(&corev1.Secret{}, "another-owner-name", "another-owner-id"),
				owner:    sampleOwner(),
			},
			wantResource: addOwner(&corev1.Secret{}, "another-owner-name", "another-owner-id"),
		},
		{
			name: "remove the single owner",
			args: args{
				resource: addOwner(&corev1.Secret{}, sampleOwner().Name, sampleOwner().UID),
				owner:    sampleOwner(),
			},
			wantResource: &corev1.Secret{ObjectMeta: metav1.ObjectMeta{OwnerReferences: []metav1.OwnerReference{}}},
		},
		{
			name: "remove the owner from a list of owners",
			args: args{
				resource: addOwner(addOwner(&corev1.Secret{}, sampleOwner().Name, sampleOwner().UID), "another-owner", "another-uid"),
				owner:    sampleOwner(),
			},
			wantResource: addOwner(&corev1.Secret{}, "another-owner", "another-uid"),
		},
		{
			name: "owner listed twice in the list (shouldn't happen): remove the first occurrence",
			args: args{
				resource: addOwner(addOwner(&corev1.Secret{}, sampleOwner().Name, sampleOwner().UID), sampleOwner().Name, sampleOwner().UID),
				owner:    sampleOwner(),
			},
			wantResource: addOwner(&corev1.Secret{}, sampleOwner().Name, sampleOwner().UID),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RemoveOwner(tt.args.resource, tt.args.owner)
			require.Equal(t, tt.wantResource, tt.args.resource)
		})
	}
}

func Test_findOwner(t *testing.T) {
	type args struct {
		resource metav1.Object
		owner    metav1.Object
	}
	tests := []struct {
		name      string
		args      args
		wantFound bool
		wantIndex int
	}{
		{
			name: "no owner: not found",
			args: args{
				resource: &corev1.Secret{},
				owner:    sampleOwner(),
			},
			wantFound: false,
			wantIndex: 0,
		},
		{
			name: "different owner: not found",
			args: args{
				resource: addOwner(&corev1.Secret{}, "another-owner-name", "another-owner-id"),
				owner:    sampleOwner(),
			},
			wantFound: false,
			wantIndex: 0,
		},
		{
			name: "owner at index 0",
			args: args{
				resource: addOwner(&corev1.Secret{}, sampleOwner().Name, sampleOwner().UID),
				owner:    sampleOwner(),
			},
			wantFound: true,
			wantIndex: 0,
		},
		{
			name: "owner at index 1",
			args: args{
				resource: addOwner(addOwner(&corev1.Secret{}, "another-owner", "another-uid"), sampleOwner().Name, sampleOwner().UID),
				owner:    sampleOwner(),
			},
			wantFound: true,
			wantIndex: 1,
		},
		{
			name: "owner listed twice in the list (shouldn't happen): return the first occurrence (index 0)",
			args: args{
				resource: addOwner(addOwner(&corev1.Secret{}, sampleOwner().Name, sampleOwner().UID), sampleOwner().Name, sampleOwner().UID),
				owner:    sampleOwner(),
			},
			wantFound: true,
			wantIndex: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotFound, gotIndex := FindOwner(tt.args.resource, tt.args.owner)
			if gotFound != tt.wantFound {
				t.Errorf("findOwner() gotFound = %v, want %v", gotFound, tt.wantFound)
			}
			if gotIndex != tt.wantIndex {
				t.Errorf("findOwner() gotIndex = %v, want %v", gotIndex, tt.wantIndex)
			}
		})
	}
}
