package nodecerts

import (
	"reflect"
	"testing"

	"github.com/elastic/k8s-operators/operators/pkg/controller/common/certificates"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func Test_shouldIssueNewCertificate(t *testing.T) {
	type args struct {
		secret corev1.Secret
		pod    corev1.Pod
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "missing cert in secret",
			args: args{
				secret: corev1.Secret{},
				pod:    testPod,
			},
			want: true,
		},
		{
			name: "invalid cert data",
			args: args{
				secret: corev1.Secret{
					Data: map[string][]byte{
						CertFileName: []byte("invalid"),
					},
				},
				pod: testPod,
			},
			want: true,
		},
		{
			name: "pod name mismatch",
			args: args{
				secret: corev1.Secret{
					Data: map[string][]byte{
						CertFileName: pemCert,
					},
				},
				pod: corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "different"}},
			},
			want: true,
		},
		{
			name: "valid cert",
			args: args{
				secret: corev1.Secret{
					Data: map[string][]byte{
						CertFileName: pemCert,
					},
				},
				pod: testPod,
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldIssueNewCertificate(tt.args.secret, testCa, tt.args.pod); got != tt.want {
				t.Errorf("shouldIssueNewCertificate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_doReconcile(t *testing.T) {
	objMeta := metav1.ObjectMeta{
		Namespace: "namespace",
		Name:      "secret",
	}
	tests := []struct {
		name              string
		secret            corev1.Secret
		pod               corev1.Pod
		wantSecretUpdated bool
	}{
		{
			name:              "no cert generated yet: issue one",
			secret:            corev1.Secret{ObjectMeta: objMeta},
			pod:               podWithRunningCertInitializer,
			wantSecretUpdated: true,
		},
		{
			name:              "no cert generated, but pod has no IP yet: requeue",
			secret:            corev1.Secret{ObjectMeta: objMeta},
			pod:               corev1.Pod{},
			wantSecretUpdated: false,
		},
		{
			name:              "no cert generated, but cert-initializer not running: requeue",
			secret:            corev1.Secret{ObjectMeta: objMeta},
			pod:               podWithTerminatedCertInitializer,
			wantSecretUpdated: false,
		},
		{
			name: "a cert already exists, is valid, and cert-initializer is not running: requeue",
			secret: corev1.Secret{
				ObjectMeta: objMeta,
				Data: map[string][]byte{
					CertFileName: pemCert,
				},
			},
			pod:               podWithTerminatedCertInitializer,
			wantSecretUpdated: false,
		},
		{
			name: "a cert already exists, is valid, and cert-initializer is running to serve a new CSR: issue cert",
			secret: corev1.Secret{
				ObjectMeta: objMeta,
				Data: map[string][]byte{
					CertFileName: pemCert,
				},
			},
			pod:               podWithRunningCertInitializer,
			wantSecretUpdated: true,
		},
		{
			name: "a cert already exists, is valid, and cert-initializer is running to serve the same CSR: requeue",
			secret: corev1.Secret{
				ObjectMeta: objMeta,
				Data: map[string][]byte{
					CertFileName: pemCert,
					CSRFileName:  testCSRBytes,
				},
			},
			pod:               podWithRunningCertInitializer,
			wantSecretUpdated: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := k8s.WrapClient(fake.NewFakeClient(&tt.secret))

			_, err := doReconcile(fakeClient, tt.secret, tt.pod, fakeCSRClient, "cluster", "namespace", []corev1.Service{testSvc}, testCa, nil)
			require.NoError(t, err)

			var updatedSecret corev1.Secret
			err = fakeClient.Get(k8s.ExtractNamespacedName(&objMeta), &updatedSecret)
			require.NoError(t, err)

			isUpdated := !reflect.DeepEqual(tt.secret, updatedSecret)
			require.Equal(t, tt.wantSecretUpdated, isUpdated)
			if tt.wantSecretUpdated {
				assert.NotEmpty(t, updatedSecret.Annotations[LastCSRUpdateAnnotation])
				assert.NotEmpty(t, updatedSecret.Data[certificates.CAFileName])
				assert.NotEmpty(t, updatedSecret.Data[CSRFileName])
				assert.NotEmpty(t, updatedSecret.Data[CertFileName])
			}
		})
	}
}
