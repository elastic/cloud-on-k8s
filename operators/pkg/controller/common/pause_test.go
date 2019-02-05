package common

import (
	"fmt"
	"testing"

	"github.com/elastic/k8s-operators/operators/pkg/apis/deployments/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	"github.com/stretchr/testify/assert"
	apiV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// Create a fake client that will return some owners
func ownerClient(sc *runtime.Scheme, ownerAnnotationSequence []map[string]string) k8s.Client {
	var stacks []runtime.Object
	for i, annotation := range ownerAnnotationSequence {
		stack := &v1alpha1.Stack{
			ObjectMeta: apiV1.ObjectMeta{
				Name:        fmt.Sprintf("a-stack-%d", i),
				Namespace:   "foo",
				Annotations: annotation,
			},
			TypeMeta: apiV1.TypeMeta{
				Kind:       "Stack",
				APIVersion: v1alpha1.SchemeGroupVersion.String(),
			},
		}
		stacks = append(stacks, stack)
	}
	return k8s.WrapClient(fake.NewFakeClientWithScheme(sc, stacks...))
}

type testcase struct {
	name string

	// annotationSequence is list of annotations that are simulated.
	annotationSequence []map[string]string

	// ownerAnnotationSequence is list of annotations that are apply on the owner (the Stack)
	ownerAnnotationSequence []map[string]string

	// Expected pause status.
	expectedState []bool
}

func registerScheme(t *testing.T) *runtime.Scheme {
	sc := scheme.Scheme
	if err := v1alpha1.SchemeBuilder.AddToScheme(sc); err != nil {
		assert.Fail(t, "failed to build custom scheme")
	}
	return sc
}

func TestPauseCondition(t *testing.T) {
	tests := []testcase{
		{
			name: "Simple pause/resume simulation (a.k.a the Happy Path)",
			annotationSequence: []map[string]string{
				{PauseAnnotationName: "true"},
				{PauseAnnotationName: "false"},
				{PauseAnnotationName: "true"},
				{PauseAnnotationName: "false"},
			},
			ownerAnnotationSequence: []map[string]string{
				{}, {}, {}, {},
			},
			expectedState: []bool{
				true,
				false,
				true,
				false,
			},
		},
		{
			name: "Can't parse or empty annotation",
			annotationSequence: []map[string]string{
				{PauseAnnotationName: ""}, // empty annotation
				{PauseAnnotationName: "true"},
				{PauseAnnotationName: "XXXX"}, // unable to parse this one
				{PauseAnnotationName: "1"},    // 1 == true
				{PauseAnnotationName: "0"},    // 0 == false
			},
			ownerAnnotationSequence: []map[string]string{
				{}, {}, {}, {},
			},
			expectedState: []bool{
				false,
				true,
				false,
				true,
				false,
			},
		},
		{
			name: "Owner (Stack) is paused",
			annotationSequence: []map[string]string{
				{PauseAnnotationName: ""},
				{PauseAnnotationName: ""},
				{PauseAnnotationName: ""},
				{PauseAnnotationName: ""},
			},
			ownerAnnotationSequence: []map[string]string{
				{PauseAnnotationName: "true"},
				{PauseAnnotationName: "false"},
				{PauseAnnotationName: "true"},
				{PauseAnnotationName: "false"},
			},
			expectedState: []bool{
				true,
				false,
				true,
				false,
			},
		},
	}

	sc := registerScheme(t)

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for i, expectedState := range test.expectedState {
				meta := apiV1.ObjectMeta{
					Name:        "bar",
					Namespace:   "foo",
					Annotations: test.annotationSequence[i],
				}
				if len(test.ownerAnnotationSequence) > 0 {
					meta.OwnerReferences = []apiV1.OwnerReference{{Kind: "Stack", Name: fmt.Sprintf("a-stack-%d", i)}}
				}
				actualPauseState := IsPaused(meta, ownerClient(sc, test.ownerAnnotationSequence))
				assert.Equal(t, expectedState, actualPauseState)
			}
		})
	}
}
