// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package common

import (
	"testing"

	"github.com/elastic/go-ucfg"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	toolsevents "k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/driver"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/settings"
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

func TestParseConfigRef(t *testing.T) {
	// any resource Kind would work here (eg. Beat, EnterpriseSearch, etc.)
	resNsn := types.NamespacedName{Namespace: "ns", Name: "resource"}
	res := corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: resNsn.Namespace, Name: resNsn.Name}}
	watchName := ConfigRefWatchName(resNsn)

	tests := []struct {
		name            string
		configRef       *commonv1.ConfigSource
		secretKey       string
		runtimeObjs     []client.Object
		want            *settings.CanonicalConfig
		wantErr         bool
		existingWatches []string
		wantWatches     []string
		wantEvent       string
	}{
		{
			name:      "happy path",
			configRef: &commonv1.ConfigSource{SecretRef: commonv1.SecretRef{SecretName: "my-secret"}},
			secretKey: "configFile.yml",
			runtimeObjs: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "my-secret"},
					Data: map[string][]byte{
						"configFile.yml": []byte("foo: bar\nbar: baz\n"),
					}},
			},
			want:        settings.MustCanonicalConfig(map[string]string{"foo": "bar", "bar": "baz"}),
			wantWatches: []string{watchName},
		},
		{
			name:      "happy path, secret already watched",
			configRef: &commonv1.ConfigSource{SecretRef: commonv1.SecretRef{SecretName: "my-secret"}},
			secretKey: "configFile.yml",
			runtimeObjs: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "my-secret"},
					Data: map[string][]byte{
						"configFile.yml": []byte("foo: bar\nbar: baz\n"),
					}},
			},
			want:            settings.MustCanonicalConfig(map[string]string{"foo": "bar", "bar": "baz"}),
			existingWatches: []string{watchName},
			wantWatches:     []string{watchName},
		},
		{
			name:      "no configRef specified",
			configRef: nil,
			secretKey: "configFile.yml",
			runtimeObjs: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "my-secret"},
					Data: map[string][]byte{
						"configFile.yml": []byte("foo: bar\nbar: baz\n"),
					}},
			},
			want:        nil,
			wantWatches: []string{},
		},
		{
			name:      "no configRef specified: clear existing watches",
			configRef: nil,
			secretKey: "configFile.yml",
			runtimeObjs: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "my-secret"},
					Data: map[string][]byte{
						"configFile.yml": []byte("foo: bar\nbar: baz\n"),
					}},
			},
			want:            nil,
			existingWatches: []string{watchName},
			wantWatches:     []string{},
		},
		{
			name:        "secret not found: error out but watch the future secret",
			configRef:   &commonv1.ConfigSource{SecretRef: commonv1.SecretRef{SecretName: "my-secret"}},
			secretKey:   "configFile.yml",
			runtimeObjs: []client.Object{},
			want:        nil,
			wantErr:     true,
			wantWatches: []string{watchName},
		},
		{
			name:      "missing key in the referenced secret: error out, watch the secret and emit an event",
			configRef: &commonv1.ConfigSource{SecretRef: commonv1.SecretRef{SecretName: "my-secret"}},
			secretKey: "configFile.yml",
			runtimeObjs: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "my-secret"},
					Data: map[string][]byte{
						"unexpected-key": []byte("foo: bar\nbar: baz\n"),
					}},
			},
			wantErr:     true,
			wantWatches: []string{watchName},
			wantEvent:   "Warning Unexpected unable to retrieve configRef secret ns/my-secret: missing key configFile.yml",
		},
		{
			name:      "invalid config the referenced secret: error out, watch the secret and emit an event",
			configRef: &commonv1.ConfigSource{SecretRef: commonv1.SecretRef{SecretName: "my-secret"}},
			secretKey: "configFile.yml",
			runtimeObjs: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "my-secret"},
					Data: map[string][]byte{
						"configFile.yml": []byte("that's not yaml"),
					}},
			},
			wantErr:     true,
			wantWatches: []string{watchName},
			wantEvent:   "Warning Unexpected unable to parse configFile.yml in configRef secret ns/my-secret",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeRecorder := toolsevents.NewFakeRecorder(10)
			w := watches.NewDynamicWatches()
			for _, existingWatch := range tt.existingWatches {
				require.NoError(t, w.Secrets.AddHandler(watches.NamedWatch[*corev1.Secret]{Name: existingWatch}))
			}
			d := fakeDriver{
				client:   k8s.NewFakeClient(tt.runtimeObjs...),
				watches:  w,
				recorder: fakeRecorder,
			}
			got, err := ParseConfigRef(d, &res, tt.configRef, tt.secretKey)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseConfigRef() error = %v, wantErr %v", err, tt.wantErr)
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

func TestParseConfigMapOrSecretRefToConfig(t *testing.T) {
	resNsn := types.NamespacedName{Namespace: "ns", Name: "resource"}
	res := corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: resNsn.Namespace, Name: resNsn.Name}}

	secretWatchName := func(nsn types.NamespacedName) string { return nsn.Namespace + "-" + nsn.Name + "-secret" }
	cmWatchName := func(nsn types.NamespacedName) string { return nsn.Namespace + "-" + nsn.Name + "-cm" }
	wantSecretWatch := secretWatchName(resNsn)
	wantCMWatch := cmWatchName(resNsn)

	tests := []struct {
		name                string
		ref                 *commonv1.ConfigMapOrSecretSource
		configMapWatchName  func(types.NamespacedName) string
		runtimeObjs         []client.Object
		want                *ucfg.Config
		wantErr             bool
		existingSecretWatch []string
		existingCMWatch     []string
		wantSecretWatches   []string
		wantCMWatches       []string
		wantEvent           string
	}{
		{
			name:                "nil ref: clears watches and returns nil",
			ref:                 nil,
			existingSecretWatch: []string{wantSecretWatch},
			existingCMWatch:     []string{wantCMWatch},
			configMapWatchName:  cmWatchName,
			wantSecretWatches:   []string{},
			wantCMWatches:       []string{},
		},
		{
			name:               "happy path - secret",
			ref:                &commonv1.ConfigMapOrSecretSource{SecretRef: commonv1.SecretRef{SecretName: "my-secret"}},
			configMapWatchName: cmWatchName,
			runtimeObjs: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "my-secret"},
					Data:       map[string][]byte{"key.yml": []byte("foo: bar\n")},
				},
			},
			wantSecretWatches: []string{wantSecretWatch},
			wantCMWatches:     []string{},
		},
		{
			name:               "happy path - configmap",
			ref:                &commonv1.ConfigMapOrSecretSource{ConfigMapRef: commonv1.ConfigMapRef{ConfigMapName: "my-cm"}},
			configMapWatchName: cmWatchName,
			runtimeObjs: []client.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "my-cm"},
					Data:       map[string]string{"key.yml": "foo: bar\n"},
				},
			},
			wantSecretWatches: []string{},
			wantCMWatches:     []string{wantCMWatch},
		},
		{
			name:               "secret not found: error but watch registered",
			ref:                &commonv1.ConfigMapOrSecretSource{SecretRef: commonv1.SecretRef{SecretName: "my-secret"}},
			configMapWatchName: cmWatchName,
			runtimeObjs:        []client.Object{},
			wantErr:            true,
			wantSecretWatches:  []string{wantSecretWatch},
			wantCMWatches:      []string{},
		},
		{
			name:               "configmap not found: error but watch registered",
			ref:                &commonv1.ConfigMapOrSecretSource{ConfigMapRef: commonv1.ConfigMapRef{ConfigMapName: "my-cm"}},
			configMapWatchName: cmWatchName,
			runtimeObjs:        []client.Object{},
			wantErr:            true,
			wantSecretWatches:  []string{},
			wantCMWatches:      []string{wantCMWatch},
		},
		{
			name:               "missing key in secret: event emitted",
			ref:                &commonv1.ConfigMapOrSecretSource{SecretRef: commonv1.SecretRef{SecretName: "my-secret"}},
			configMapWatchName: cmWatchName,
			runtimeObjs: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "my-secret"},
					Data:       map[string][]byte{"other.yml": []byte("foo: bar\n")},
				},
			},
			wantErr:           true,
			wantSecretWatches: []string{wantSecretWatch},
			wantCMWatches:     []string{},
			wantEvent:         "Warning Unexpected unable to retrieve myRef secret ns/my-secret: missing key key.yml",
		},
		{
			name:               "missing key in configmap: event emitted",
			ref:                &commonv1.ConfigMapOrSecretSource{ConfigMapRef: commonv1.ConfigMapRef{ConfigMapName: "my-cm"}},
			configMapWatchName: cmWatchName,
			runtimeObjs: []client.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "my-cm"},
					Data:       map[string]string{"other.yml": "foo: bar\n"},
				},
			},
			wantErr:           true,
			wantSecretWatches: []string{},
			wantCMWatches:     []string{wantCMWatch},
			wantEvent:         "Warning Unexpected unable to retrieve myRef configmap ns/my-cm: missing key key.yml",
		},
		{
			name:               "invalid yaml in secret: event emitted",
			ref:                &commonv1.ConfigMapOrSecretSource{SecretRef: commonv1.SecretRef{SecretName: "my-secret"}},
			configMapWatchName: cmWatchName,
			runtimeObjs: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "my-secret"},
					Data:       map[string][]byte{"key.yml": []byte("not: valid: yaml")},
				},
			},
			wantErr:           true,
			wantSecretWatches: []string{wantSecretWatch},
			wantCMWatches:     []string{},
			wantEvent:         "Warning Unexpected unable to parse key.yml in myRef secret ns/my-secret",
		},
		{
			name:               "invalid yaml in configmap: event emitted",
			ref:                &commonv1.ConfigMapOrSecretSource{ConfigMapRef: commonv1.ConfigMapRef{ConfigMapName: "my-cm"}},
			configMapWatchName: cmWatchName,
			runtimeObjs: []client.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "my-cm"},
					Data:       map[string]string{"key.yml": "not: valid: yaml"},
				},
			},
			wantErr:           true,
			wantSecretWatches: []string{},
			wantCMWatches:     []string{wantCMWatch},
			wantEvent:         "Warning Unexpected unable to parse key.yml in myRef configmap ns/my-cm",
		},
		{
			name:                "switch from secret to configmap: secret watch cleared",
			ref:                 &commonv1.ConfigMapOrSecretSource{ConfigMapRef: commonv1.ConfigMapRef{ConfigMapName: "my-cm"}},
			configMapWatchName:  cmWatchName,
			existingSecretWatch: []string{wantSecretWatch},
			runtimeObjs: []client.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "my-cm"},
					Data:       map[string]string{"key.yml": "foo: bar\n"},
				},
			},
			wantSecretWatches: []string{},
			wantCMWatches:     []string{wantCMWatch},
		},
		{
			name:               "switch from configmap to secret: configmap watch cleared",
			ref:                &commonv1.ConfigMapOrSecretSource{SecretRef: commonv1.SecretRef{SecretName: "my-secret"}},
			configMapWatchName: cmWatchName,
			existingCMWatch:    []string{wantCMWatch},
			runtimeObjs: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "my-secret"},
					Data:       map[string][]byte{"key.yml": []byte("foo: bar\n")},
				},
			},
			wantSecretWatches: []string{wantSecretWatch},
			wantCMWatches:     []string{},
		},
		{
			name:               "nil configMapWatchName: configmap watch skipped",
			ref:                &commonv1.ConfigMapOrSecretSource{SecretRef: commonv1.SecretRef{SecretName: "my-secret"}},
			configMapWatchName: nil,
			runtimeObjs: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "my-secret"},
					Data:       map[string][]byte{"key.yml": []byte("foo: bar\n")},
				},
			},
			wantSecretWatches: []string{wantSecretWatch},
			wantCMWatches:     []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeRecorder := toolsevents.NewFakeRecorder(10)
			w := watches.NewDynamicWatches()
			for _, name := range tt.existingSecretWatch {
				require.NoError(t, w.Secrets.AddHandler(watches.NamedWatch[*corev1.Secret]{Name: name}))
			}
			for _, name := range tt.existingCMWatch {
				require.NoError(t, w.ConfigMaps.AddHandler(watches.NamedWatch[*corev1.ConfigMap]{Name: name}))
			}
			d := fakeDriver{
				client:   k8s.NewFakeClient(tt.runtimeObjs...),
				watches:  w,
				recorder: fakeRecorder,
			}
			_, err := ParseConfigMapOrSecretRefToConfig(d, &res, tt.ref, "key.yml", "myRef", secretWatchName, tt.configMapWatchName, settings.Options)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseConfigMapOrSecretRefToConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			require.Equal(t, tt.wantSecretWatches, d.watches.Secrets.Registrations())
			require.Equal(t, tt.wantCMWatches, d.watches.ConfigMaps.Registrations())

			if tt.wantEvent != "" {
				require.Equal(t, tt.wantEvent, <-fakeRecorder.Events)
			} else {
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
