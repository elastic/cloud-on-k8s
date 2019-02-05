package k8s

import (
	"testing"

	"github.com/elastic/k8s-operators/local-volume/pkg/provider"
	"github.com/stretchr/testify/assert"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/kubernetes/pkg/kubelet/apis"
)

func TestClient_BindPVToNode(t *testing.T) {
	// fixtures
	pvName := "pv-test"
	nodeName := "node-test"
	emptyPV := NewPersistentVolume(pvName)
	upToDatePV := NewPersistentVolume(pvName)
	updatePVLabelsForNode(upToDatePV, nodeName)
	updatePVNodeAffinity(upToDatePV, nodeName)

	type args struct {
		pvName   string
		nodeName string
	}
	tests := []struct {
		name              string
		args              args
		wantErr           bool
		existingResources []runtime.Object
		shouldUpdate      bool
	}{
		{
			name: "out-of-date PV should be updated",
			args: args{
				pvName:   pvName,
				nodeName: nodeName,
			},
			wantErr:           false,
			existingResources: []runtime.Object{emptyPV},
			shouldUpdate:      true,
		},
		{
			name: "up-to-date PV should not be updated",
			args: args{
				pvName:   pvName,
				nodeName: nodeName,
			},
			wantErr:           false,
			existingResources: []runtime.Object{upToDatePV},
			shouldUpdate:      false,
		},
		{
			name: "non-existing PV should return an error",
			args: args{
				pvName:   pvName,
				nodeName: nodeName,
			},
			wantErr:           true,
			existingResources: []runtime.Object{},
			shouldUpdate:      false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewTestClient(tt.existingResources...)
			updated, err := c.BindPVToNode(tt.args.pvName, tt.args.nodeName)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.shouldUpdate, updated)
			pv, err := c.ClientSet.CoreV1().PersistentVolumes().Get(pvName, metav1.GetOptions{})
			assert.NoError(t, err)
			// make sure the PV node affinity was updated
			expectedAffinity := v1.NodeSelectorRequirement{
				Key:      apis.LabelHostname,
				Operator: v1.NodeSelectorOpIn,
				Values:   []string{tt.args.nodeName},
			}
			assert.Equal(t, pv.Spec.NodeAffinity.Required.NodeSelectorTerms[0].MatchExpressions[0], expectedAffinity)
			// make sure the label was updated
			expectedLabel := nodeName
			actualLabel := pv.Labels[provider.NodeAffinityLabel]
			assert.Equal(t, expectedLabel, actualLabel)
		})
	}
}
