// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package pipelines

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/driver"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

type fakeDriver struct {
	client   k8s.Client
	watches  watches.DynamicWatches
	recorder record.EventRecorder
}

func (f fakeDriver) K8sClient() k8s.Client {
	return f.client
}

func (f fakeDriver) DynamicWatches() watches.DynamicWatches {
	return f.watches
}

func (f fakeDriver) Recorder() record.EventRecorder {
	return f.recorder
}

var _ driver.Interface = fakeDriver{}

func TestParsePipelinesRef(t *testing.T) {
	// any resource Kind would work here (eg. Beat, EnterpriseSearch, etc.)
	resNsn := types.NamespacedName{Namespace: "ns", Name: "resource"}
	res := corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: resNsn.Namespace, Name: resNsn.Name}}
	watchName := RefWatchName(resNsn)

	tests := []struct {
		name            string
		pipelinesRef    *commonv1.ConfigSource
		secretKey       string
		runtimeObjs     []client.Object
		want            *Config
		wantErr         bool
		existingWatches []string
		wantWatches     []string
		wantEvent       string
	}{
		{
			name:         "happy path",
			pipelinesRef: &commonv1.ConfigSource{SecretRef: commonv1.SecretRef{SecretName: "my-secret"}},
			secretKey:    "configFile.yml",
			runtimeObjs: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "my-secret"},
					Data: map[string][]byte{
						"configFile.yml": []byte(`- "pipeline.id": "main"`),
					}},
			},
			want:        MustParse([]byte(`- "pipeline.id": "main"`)),
			wantWatches: []string{watchName},
		},
		{
			name:         "happy path, secret already watched",
			pipelinesRef: &commonv1.ConfigSource{SecretRef: commonv1.SecretRef{SecretName: "my-secret"}},
			secretKey:    "configFile.yml",
			runtimeObjs: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "my-secret"},
					Data: map[string][]byte{
						"configFile.yml": []byte(`- "pipeline.id": "main"`),
					}},
			},
			want:            MustParse([]byte(`- "pipeline.id": "main"`)),
			existingWatches: []string{watchName},
			wantWatches:     []string{watchName},
		},
		{
			name:         "no pipelinesRef specified",
			pipelinesRef: nil,
			secretKey:    "configFile.yml",
			runtimeObjs: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "my-secret"},
					Data: map[string][]byte{
						"configFile.yml": []byte(`- "pipeline.id": "main"`),
					}},
			},
			want:        nil,
			wantWatches: []string{},
		},
		{
			name:         "no pipelinesRef specified: clear existing watches",
			pipelinesRef: nil,
			secretKey:    "configFile.yml",
			runtimeObjs: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "my-secret"},
					Data: map[string][]byte{
						"configFile.yml": []byte(`- "pipeline.id": "main"`),
					}},
			},
			want:            nil,
			existingWatches: []string{watchName},
			wantWatches:     []string{},
		},
		{
			name:         "secret not found: error out but watch the future secret",
			pipelinesRef: &commonv1.ConfigSource{SecretRef: commonv1.SecretRef{SecretName: "my-secret"}},
			secretKey:    "configFile.yml",
			runtimeObjs:  []client.Object{},
			want:         nil,
			wantErr:      true,
			wantWatches:  []string{watchName},
		},
		{
			name:         "missing key in the referenced secret: error out, watch the secret and emit an event",
			pipelinesRef: &commonv1.ConfigSource{SecretRef: commonv1.SecretRef{SecretName: "my-secret"}},
			secretKey:    "configFile.yml",
			runtimeObjs: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "my-secret"},
					Data: map[string][]byte{
						"unexpected-key": []byte(`- "pipeline.id": "main"`),
					}},
			},
			wantErr:     true,
			wantWatches: []string{watchName},
			wantEvent:   "Warning Unexpected unable to parse configRef secret ns/my-secret: missing key configFile.yml",
		},
		{
			name:         "invalid config the referenced secret: error out, watch the secret and emit an event",
			pipelinesRef: &commonv1.ConfigSource{SecretRef: commonv1.SecretRef{SecretName: "my-secret"}},
			secretKey:    "configFile.yml",
			runtimeObjs: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "my-secret"},
					Data: map[string][]byte{
						"configFile.yml": []byte("this.is invalid config"),
					}},
			},
			wantErr:     true,
			wantWatches: []string{watchName},
			wantEvent:   "Warning Unexpected unable to parse configFile.yml in configRef secret ns/my-secret",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeRecorder := record.NewFakeRecorder(10)
			w := watches.NewDynamicWatches()
			for _, existingWatch := range tt.existingWatches {
				require.NoError(t, w.Secrets.AddHandler(watches.NamedWatch[*corev1.Secret]{Name: existingWatch}))
			}
			d := fakeDriver{
				client:   k8s.NewFakeClient(tt.runtimeObjs...),
				watches:  w,
				recorder: fakeRecorder,
			}
			got, err := ParsePipelinesRef(d, &res, tt.pipelinesRef, tt.secretKey)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParsePipelinesRef() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			require.Equal(t, tt.want, got)
			require.Equal(t, tt.wantWatches, d.watches.Secrets.Registrations())

			if tt.wantEvent != "" {
				require.Equal(t, tt.wantEvent, <-fakeRecorder.Events)
			} else {
				// no event expected
				select {
				case e := <-fakeRecorder.Events:
					require.Fail(t, "no event expected but got one", "event", e)
				default:
					// ok
				}
			}
		})
	}
}
