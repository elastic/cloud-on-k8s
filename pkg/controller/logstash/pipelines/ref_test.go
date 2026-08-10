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
	toolsevents "k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/driver"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

type fakeDriver struct {
	client   k8s.Client
	watches  watches.DynamicWatches
	recorder toolsevents.EventRecorder
}

func (f fakeDriver) K8sClient() k8s.Client {
	return f.client
}

func (f fakeDriver) DynamicWatches() watches.DynamicWatches {
	return f.watches
}

func (f fakeDriver) Recorder() toolsevents.EventRecorder {
	return f.recorder
}

var _ driver.Interface = fakeDriver{}

func TestParsePipelinesRef(t *testing.T) {
	// any resource Kind would work here (eg. Beat, EnterpriseSearch, etc.)
	resNsn := types.NamespacedName{Namespace: "ns", Name: "resource"}
	res := corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: resNsn.Namespace, Name: resNsn.Name}}
	secretWatchName := SecretRefWatchName(resNsn)
	cmWatchName := ConfigMapRefWatchName(resNsn)

	tests := []struct {
		name                  string
		pipelinesRef          *commonv1.ConfigMapOrSecretSource
		secretKey             string
		runtimeObjs           []client.Object
		want                  *Config
		wantErr               bool
		existingSecretWatches []string
		existingCMWatches     []string
		wantSecretWatches     []string
		wantCMWatches         []string
		wantEvent             string
	}{
		{
			name:         "happy path - secret",
			pipelinesRef: &commonv1.ConfigMapOrSecretSource{SecretRef: commonv1.SecretRef{SecretName: "my-secret"}},
			secretKey:    "configFile.yml",
			runtimeObjs: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "my-secret"},
					Data: map[string][]byte{
						"configFile.yml": []byte(`- "pipeline.id": "main"`),
					}},
			},
			want:              MustParse([]byte(`- "pipeline.id": "main"`)),
			wantSecretWatches: []string{secretWatchName},
			wantCMWatches:     []string{},
		},
		{
			name:         "happy path - secret already watched",
			pipelinesRef: &commonv1.ConfigMapOrSecretSource{SecretRef: commonv1.SecretRef{SecretName: "my-secret"}},
			secretKey:    "configFile.yml",
			runtimeObjs: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "my-secret"},
					Data: map[string][]byte{
						"configFile.yml": []byte(`- "pipeline.id": "main"`),
					}},
			},
			want:                  MustParse([]byte(`- "pipeline.id": "main"`)),
			existingSecretWatches: []string{secretWatchName},
			wantSecretWatches:     []string{secretWatchName},
			wantCMWatches:         []string{},
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
			want:              nil,
			wantSecretWatches: []string{},
			wantCMWatches:     []string{},
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
			want:                  nil,
			existingSecretWatches: []string{secretWatchName},
			existingCMWatches:     []string{cmWatchName},
			wantSecretWatches:     []string{},
			wantCMWatches:         []string{},
		},
		{
			name:              "secret not found: error out but watch the future secret",
			pipelinesRef:      &commonv1.ConfigMapOrSecretSource{SecretRef: commonv1.SecretRef{SecretName: "my-secret"}},
			secretKey:         "configFile.yml",
			runtimeObjs:       []client.Object{},
			want:              nil,
			wantErr:           true,
			wantSecretWatches: []string{secretWatchName},
			wantCMWatches:     []string{},
		},
		{
			name:         "missing key in the referenced secret: error out, watch the secret and emit an event",
			pipelinesRef: &commonv1.ConfigMapOrSecretSource{SecretRef: commonv1.SecretRef{SecretName: "my-secret"}},
			secretKey:    "configFile.yml",
			runtimeObjs: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "my-secret"},
					Data: map[string][]byte{
						"unexpected-key": []byte(`- "pipeline.id": "main"`),
					}},
			},
			wantErr:           true,
			wantSecretWatches: []string{secretWatchName},
			wantCMWatches:     []string{},
			wantEvent:         "Warning Unexpected unable to retrieve pipelinesRef secret ns/my-secret: missing key configFile.yml",
		},
		{
			name:         "invalid config in the referenced secret: error out, watch the secret and emit an event",
			pipelinesRef: &commonv1.ConfigMapOrSecretSource{SecretRef: commonv1.SecretRef{SecretName: "my-secret"}},
			secretKey:    "configFile.yml",
			runtimeObjs: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "my-secret"},
					Data: map[string][]byte{
						"configFile.yml": []byte("this.is invalid config"),
					}},
			},
			wantErr:           true,
			wantSecretWatches: []string{secretWatchName},
			wantCMWatches:     []string{},
			wantEvent:         "Warning Unexpected unable to parse configFile.yml in pipelinesRef secret ns/my-secret",
		},
		{
			name:         "happy path - configmap",
			pipelinesRef: &commonv1.ConfigMapOrSecretSource{ConfigMapRef: commonv1.ConfigMapRef{ConfigMapName: "my-cm"}},
			secretKey:    "configFile.yml",
			runtimeObjs: []client.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "my-cm"},
					Data: map[string]string{
						"configFile.yml": `- "pipeline.id": "main"`,
					}},
			},
			want:              MustParse([]byte(`- "pipeline.id": "main"`)),
			wantSecretWatches: []string{},
			wantCMWatches:     []string{cmWatchName},
		},
		{
			name:              "configmap not found: error out but watch the future configmap",
			pipelinesRef:      &commonv1.ConfigMapOrSecretSource{ConfigMapRef: commonv1.ConfigMapRef{ConfigMapName: "my-cm"}},
			secretKey:         "configFile.yml",
			runtimeObjs:       []client.Object{},
			want:              nil,
			wantErr:           true,
			wantSecretWatches: []string{},
			wantCMWatches:     []string{cmWatchName},
		},
		{
			name:         "missing key in the referenced configmap: error out, watch the configmap and emit an event",
			pipelinesRef: &commonv1.ConfigMapOrSecretSource{ConfigMapRef: commonv1.ConfigMapRef{ConfigMapName: "my-cm"}},
			secretKey:    "configFile.yml",
			runtimeObjs: []client.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "my-cm"},
					Data: map[string]string{
						"unexpected-key": `- "pipeline.id": "main"`,
					}},
			},
			wantErr:           true,
			wantSecretWatches: []string{},
			wantCMWatches:     []string{cmWatchName},
			wantEvent:         "Warning Unexpected unable to retrieve pipelinesRef configmap ns/my-cm: missing key configFile.yml",
		},
		{
			name:         "invalid config in the referenced configmap: error out, watch the configmap and emit an event",
			pipelinesRef: &commonv1.ConfigMapOrSecretSource{ConfigMapRef: commonv1.ConfigMapRef{ConfigMapName: "my-cm"}},
			secretKey:    "configFile.yml",
			runtimeObjs: []client.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "my-cm"},
					Data: map[string]string{
						"configFile.yml": "this.is invalid config",
					}},
			},
			wantErr:           true,
			wantSecretWatches: []string{},
			wantCMWatches:     []string{cmWatchName},
			wantEvent:         "Warning Unexpected unable to parse configFile.yml in pipelinesRef configmap ns/my-cm",
		},
		{
			name:         "switch from secret to configmap: secret watch cleared",
			pipelinesRef: &commonv1.ConfigMapOrSecretSource{ConfigMapRef: commonv1.ConfigMapRef{ConfigMapName: "my-cm"}},
			secretKey:    "configFile.yml",
			runtimeObjs: []client.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "my-cm"},
					Data: map[string]string{
						"configFile.yml": `- "pipeline.id": "main"`,
					}},
			},
			existingSecretWatches: []string{secretWatchName},
			want:                  MustParse([]byte(`- "pipeline.id": "main"`)),
			wantSecretWatches:     []string{},
			wantCMWatches:         []string{cmWatchName},
		},
		{
			name:         "switch from configmap to secret: configmap watch cleared",
			pipelinesRef: &commonv1.ConfigMapOrSecretSource{SecretRef: commonv1.SecretRef{SecretName: "my-secret"}},
			secretKey:    "configFile.yml",
			runtimeObjs: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "my-secret"},
					Data: map[string][]byte{
						"configFile.yml": []byte(`- "pipeline.id": "main"`),
					}},
			},
			existingCMWatches: []string{cmWatchName},
			want:              MustParse([]byte(`- "pipeline.id": "main"`)),
			wantSecretWatches: []string{secretWatchName},
			wantCMWatches:     []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeRecorder := toolsevents.NewFakeRecorder(10)
			w := watches.NewDynamicWatches()
			for _, existingWatch := range tt.existingSecretWatches {
				require.NoError(t, w.Secrets.AddHandler(watches.NamedWatch[*corev1.Secret]{Name: existingWatch}))
			}
			for _, existingWatch := range tt.existingCMWatches {
				require.NoError(t, w.ConfigMaps.AddHandler(watches.NamedWatch[*corev1.ConfigMap]{Name: existingWatch}))
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
			require.Equal(t, tt.wantSecretWatches, d.watches.Secrets.Registrations())
			require.Equal(t, tt.wantCMWatches, d.watches.ConfigMaps.Registrations())

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
