// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package logstash

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	toolsevents "k8s.io/client-go/tools/events"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	logstashv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/logstash/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/expectations"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/logstash/labels"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

func selfSignedTLS(disabled bool, sans ...commonv1.SubjectAlternativeName) commonv1.TLSOptions {
	return commonv1.TLSOptions{
		SelfSignedCertificate: &commonv1.SelfSignedCertificate{Disabled: disabled, SubjectAlternativeNames: sans},
	}
}

func Test_certificateTLSOptions(t *testing.T) {
	customCert := commonv1.TLSOptions{Certificate: commonv1.SecretRef{SecretName: "my-cert"}}
	apiSANs := selfSignedTLS(false, commonv1.SubjectAlternativeName{DNS: "api.example.com"})

	tests := []struct {
		name     string
		services []logstashv1alpha1.LogstashService
		want     commonv1.TLSOptions
	}{
		{
			name: "no service: default TLS options of the API Service",
			want: commonv1.TLSOptions{},
		},
		{
			name: "API Service self-signed, contributing Service: TLS options of the API Service",
			services: []logstashv1alpha1.LogstashService{
				{Name: "api", TLS: apiSANs},
				{Name: "beats", TLS: selfSignedTLS(false, commonv1.SubjectAlternativeName{DNS: "beats.example.com"})},
			},
			want: apiSANs,
		},
		{
			name: "API Service with a custom certificate, contributing Service: TLS options of the API Service",
			services: []logstashv1alpha1.LogstashService{
				{Name: "api", TLS: customCert},
				{Name: "beats", TLS: selfSignedTLS(false)},
			},
			want: customCert,
		},
		{
			name: "API Service TLS disabled, no contributing Service: TLS disabled",
			services: []logstashv1alpha1.LogstashService{
				{Name: "api", TLS: selfSignedTLS(true)},
				{Name: "tcp", TLS: selfSignedTLS(true)},
			},
			want: selfSignedTLS(true),
		},
		{
			name: "API Service TLS disabled, Service without TLS options: self-signed certificate",
			services: []logstashv1alpha1.LogstashService{
				{Name: "api", TLS: selfSignedTLS(true)},
				{Name: "beats"},
			},
			want: selfSignedTLS(false),
		},
		{
			name: "API Service TLS disabled, contributing Service: self-signed certificate",
			services: []logstashv1alpha1.LogstashService{
				{Name: "api", TLS: selfSignedTLS(true)},
				{Name: "beats", TLS: selfSignedTLS(false, commonv1.SubjectAlternativeName{DNS: "beats.example.com"})},
			},
			want: selfSignedTLS(false),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ls := logstashv1alpha1.Logstash{
				Name: "ls", Namespace: "test",
				Spec: logstashv1alpha1.LogstashSpec{Services: tt.services},
			}
			assert.Equal(t, tt.want, certificateTLSOptions(ls))
		})
	}
}

func Test_certificateServices(t *testing.T) {
	svc := func(name string) corev1.Service {
		return corev1.Service{Name: name, Namespace: "test"}
	}
	tests := []struct {
		name       string
		services   []logstashv1alpha1.LogstashService
		reconciled []corev1.Service
		want       []string
	}{
		{
			name:       "default API Service only",
			reconciled: []corev1.Service{svc("ls-ls-api")},
			want:       []string{"ls-ls-api"},
		},
		{
			name: "Services with a disabled self-signed certificate are not included",
			services: []logstashv1alpha1.LogstashService{
				{Name: "beats"},
				{Name: "tcp", TLS: selfSignedTLS(true)},
				{Name: "udp", TLS: commonv1.TLSOptions{Certificate: commonv1.SecretRef{SecretName: "my-cert"}}},
			},
			reconciled: []corev1.Service{svc("ls-ls-beats"), svc("ls-ls-tcp"), svc("ls-ls-udp"), svc("ls-ls-api")},
			want:       []string{"ls-ls-api", "ls-ls-beats", "ls-ls-udp"},
		},
		{
			name: "API Service first, then contributing Services in the order of the spec",
			services: []logstashv1alpha1.LogstashService{
				{Name: "tcp", TLS: selfSignedTLS(false)},
				{Name: "disabled", TLS: selfSignedTLS(true)},
				{Name: "api", TLS: selfSignedTLS(false)},
				{Name: "beats", TLS: selfSignedTLS(false)},
			},
			reconciled: []corev1.Service{svc("ls-ls-tcp"), svc("ls-ls-disabled"), svc("ls-ls-api"), svc("ls-ls-beats")},
			want:       []string{"ls-ls-api", "ls-ls-tcp", "ls-ls-beats"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ls := logstashv1alpha1.Logstash{
				Name: "ls", Namespace: "test",
				Spec: logstashv1alpha1.LogstashSpec{Services: tt.services},
			}
			var got []string
			for _, s := range certificateServices(ls, svc("ls-ls-api"), tt.reconciled) {
				got = append(got, s.Name)
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

// certificateTestEnv runs internalReconcile against a fake client and gives access to the resulting resources.
type certificateTestEnv struct {
	t            *testing.T
	client       k8s.Client
	expectations *expectations.ClustersExpectation
	watches      watches.DynamicWatches
}

func newCertificateTestEnv(t *testing.T) *certificateTestEnv {
	t.Helper()
	client := k8s.NewFakeClient()
	return &certificateTestEnv{
		t:            t,
		client:       client,
		expectations: expectations.NewClustersExpectations(client, &appsv1.StatefulSet{}),
		watches:      watches.NewDynamicWatches("testCtr"),
	}
}

func (e *certificateTestEnv) reconcile(services []logstashv1alpha1.LogstashService) {
	e.t.Helper()
	ls := logstashv1alpha1.Logstash{
		Name: "ls", Namespace: "test",
		Spec: logstashv1alpha1.LogstashSpec{
			Version: "8.12.0",
			// The fake client does not run the StatefulSet controller, which means that no Pod is created.
			// Without replicas, no Pod creation is pending and consecutive reconciliations update the StatefulSet.
			Count:    0,
			Services: services,
		},
	}
	rotation := certificates.RotationParams{Validity: 365 * 24 * time.Hour, RotateBefore: 24 * time.Hour}
	results, _ := internalReconcile(Params{
		Context:       context.Background(),
		Meta:          metadata.Metadata{Labels: labels.NewLabels(ls)},
		Client:        e.client,
		EventRecorder: toolsevents.NewFakeRecorder(100),
		Watches:       e.watches,
		Logstash:      ls,
		Version:       version.MustParse(ls.Spec.Version),
		OperatorParams: operator.Parameters{
			CACertRotation: rotation,
			CertRotation:   rotation,
		},
		Expectations: e.expectations.ForCluster(k8s.ExtractNamespacedName(&ls)),
	})
	_, err := results.Aggregate()
	require.NoError(e.t, err)
}

// certSecret returns the internal HTTP certificates Secret, or nil if it does not exist.
func (e *certificateTestEnv) certSecret() *corev1.Secret {
	e.t.Helper()
	var secret corev1.Secret
	err := e.client.Get(context.Background(), types.NamespacedName{
		Namespace: "test",
		Name:      certificates.InternalCertsSecretName(logstashv1alpha1.Namer, "ls"),
	}, &secret)
	if apierrors.IsNotFound(err) {
		return nil
	}
	require.NoError(e.t, err)
	return &secret
}

// certSANs returns the DNS names and the IP addresses of the HTTP certificate.
func (e *certificateTestEnv) certSANs() ([]string, []string) {
	e.t.Helper()
	secret := e.certSecret()
	require.NotNil(e.t, secret, "the HTTP certificates Secret must exist")
	cert, err := certificates.GetPrimaryCertificate(secret.Data[certificates.CertFileName])
	require.NoError(e.t, err)
	ips := make([]string, 0, len(cert.IPAddresses))
	for _, ip := range cert.IPAddresses {
		ips = append(ips, ip.String())
	}
	return cert.DNSNames, ips
}

func (e *certificateTestEnv) podTemplate() corev1.PodTemplateSpec {
	e.t.Helper()
	var sset appsv1.StatefulSet
	require.NoError(e.t, e.client.Get(context.Background(), types.NamespacedName{
		Namespace: "test",
		Name:      logstashv1alpha1.Name("ls"),
	}, &sset))
	return sset.Spec.Template
}

func hasHTTPCertsVolumeMount(podTemplate corev1.PodTemplateSpec) bool {
	for _, mount := range GetLogstashContainer(podTemplate.Spec).VolumeMounts {
		if mount.Name == certificates.HTTPCertificatesSecretVolumeName &&
			mount.MountPath == certificates.HTTPCertificatesSecretVolumeMountPath {
			return true
		}
	}
	return false
}

func Test_internalReconcile_certificate(t *testing.T) {
	// SANs of a Logstash named "ls" in the namespace "test", when no Service contributes to the certificate.
	defaultDNSNames := []string{
		"ls-ls-http.test.ls.local",
		"ls-ls-http",
		"ls-ls-api.test.svc",
		"ls-ls-api.test",
	}
	beatsPorts := []corev1.ServicePort{{Name: "beats", Port: 5044, Protocol: corev1.ProtocolTCP}}
	beatsService := func(tls commonv1.TLSOptions) logstashv1alpha1.LogstashService {
		return logstashv1alpha1.LogstashService{
			Name: "beats",
			Service: commonv1.ServiceTemplate{Spec: corev1.ServiceSpec{
				Ports:       beatsPorts,
				ExternalIPs: []string{"10.11.12.13"},
			}},
			TLS: tls,
		}
	}
	beatsSANs := []commonv1.SubjectAlternativeName{
		{DNS: "logstash.example.com"},
		{IP: "1.2.3.4"},
	}

	tests := []struct {
		name         string
		services     []logstashv1alpha1.LogstashService
		wantSecret   bool
		wantDNSNames []string
		wantIPs      []string
		wantMount    bool
		wantUseTLS   string
	}{
		{
			name:         "no Service in the spec",
			wantSecret:   true,
			wantDNSNames: defaultDNSNames,
			wantMount:    true,
			wantUseTLS:   "true",
		},
		{
			name:       "a Service without TLS options adds its names",
			services:   []logstashv1alpha1.LogstashService{beatsService(commonv1.TLSOptions{})},
			wantSecret: true,
			wantDNSNames: append(append([]string{}, defaultDNSNames...),
				"ls-ls-beats.test.svc",
				"ls-ls-beats.test",
			),
			wantIPs:    []string{"10.11.12.13"},
			wantMount:  true,
			wantUseTLS: "true",
		},
		{
			name:         "a Service with a disabled self-signed certificate does not change the certificate",
			services:     []logstashv1alpha1.LogstashService{beatsService(selfSignedTLS(true, beatsSANs...))},
			wantSecret:   true,
			wantDNSNames: defaultDNSNames,
			wantMount:    true,
			wantUseTLS:   "true",
		},
		{
			name:       "a Service with a self-signed certificate adds its names and its SANs",
			services:   []logstashv1alpha1.LogstashService{beatsService(selfSignedTLS(false, beatsSANs...))},
			wantSecret: true,
			wantDNSNames: append(append([]string{}, defaultDNSNames...),
				"ls-ls-beats.test.svc",
				"ls-ls-beats.test",
				"logstash.example.com",
			),
			wantIPs:    []string{"10.11.12.13", "1.2.3.4"},
			wantMount:  true,
			wantUseTLS: "true",
		},
		{
			name: "SANs of the API Service come before the SANs of the other Services",
			services: []logstashv1alpha1.LogstashService{
				beatsService(selfSignedTLS(false, beatsSANs...)),
				{Name: "api", TLS: selfSignedTLS(false, commonv1.SubjectAlternativeName{DNS: "api.example.com"})},
			},
			wantSecret: true,
			wantDNSNames: append(append([]string{}, defaultDNSNames...),
				"ls-ls-beats.test.svc",
				"ls-ls-beats.test",
				"api.example.com",
				"logstash.example.com",
			),
			wantIPs:    []string{"10.11.12.13", "1.2.3.4"},
			wantMount:  true,
			wantUseTLS: "true",
		},
		{
			name: "TLS disabled for the API Service and no contributing Service: no certificate",
			services: []logstashv1alpha1.LogstashService{
				{Name: "api", TLS: selfSignedTLS(true)},
				beatsService(selfSignedTLS(true, beatsSANs...)),
			},
			wantSecret: false,
			wantMount:  false,
			wantUseTLS: "false",
		},
		{
			name: "TLS disabled for the API Service and a contributing Service: certificate for the other Services only",
			services: []logstashv1alpha1.LogstashService{
				{Name: "api", TLS: selfSignedTLS(true)},
				beatsService(selfSignedTLS(false, beatsSANs...)),
			},
			wantSecret: true,
			wantDNSNames: append(append([]string{}, defaultDNSNames...),
				"ls-ls-beats.test.svc",
				"ls-ls-beats.test",
				"logstash.example.com",
			),
			wantIPs:    []string{"10.11.12.13", "1.2.3.4"},
			wantMount:  true,
			wantUseTLS: "false",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newCertificateTestEnv(t)
			env.reconcile(tt.services)

			if tt.wantSecret {
				dnsNames, ips := env.certSANs()
				assert.Equal(t, tt.wantDNSNames, dnsNames)
				if tt.wantIPs == nil {
					tt.wantIPs = []string{}
				}
				assert.Equal(t, tt.wantIPs, ips)
			} else {
				assert.Nil(t, env.certSecret())
			}

			podTemplate := env.podTemplate()
			assert.Equal(t, tt.wantMount, hasHTTPCertsVolumeMount(podTemplate))
			// The API server keystore is only prepared when TLS is enabled for the API Service.
			initContainer := GetConfigInitContainer(podTemplate.Spec)
			require.NotNil(t, initContainer)
			useTLS := GetEnvByName(initContainer.Env, UseTLSEnv)
			require.NotNil(t, useTLS)
			assert.Equal(t, tt.wantUseTLS, useTLS.Value)
			assert.Equal(t, tt.wantUseTLS == "true", GetEnvByName(initContainer.Env, APIKeystorePassEnv) != nil)
		})
	}
}

// Test_internalReconcile_certificateUpdates checks that the certificate, the volume mount and the config hash follow
// the changes of the Services which contribute to the certificate.
func Test_internalReconcile_certificateUpdates(t *testing.T) {
	apiDisabled := logstashv1alpha1.LogstashService{Name: "api", TLS: selfSignedTLS(true)}
	beats := func(tls commonv1.TLSOptions) logstashv1alpha1.LogstashService {
		return logstashv1alpha1.LogstashService{
			Name: "beats",
			Service: commonv1.ServiceTemplate{Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{{Name: "beats", Port: 5044, Protocol: corev1.ProtocolTCP}},
			}},
			TLS: tls,
		}
	}

	t.Run("a new SAN issues a new certificate and changes the config hash", func(t *testing.T) {
		env := newCertificateTestEnv(t)

		env.reconcile([]logstashv1alpha1.LogstashService{beats(commonv1.TLSOptions{})})
		initialCert := env.certSecret().Data[certificates.CertFileName]
		initialHash := env.podTemplate().Annotations[ConfigHashAnnotationName]

		// a second reconciliation of the same spec keeps the certificate
		env.reconcile([]logstashv1alpha1.LogstashService{beats(commonv1.TLSOptions{})})
		assert.Equal(t, initialCert, env.certSecret().Data[certificates.CertFileName])
		assert.Equal(t, initialHash, env.podTemplate().Annotations[ConfigHashAnnotationName])

		env.reconcile([]logstashv1alpha1.LogstashService{
			beats(selfSignedTLS(false, commonv1.SubjectAlternativeName{DNS: "logstash.example.com"})),
		})
		dnsNames, _ := env.certSANs()
		assert.Contains(t, dnsNames, "logstash.example.com")
		assert.NotEqual(t, initialCert, env.certSecret().Data[certificates.CertFileName])
		assert.NotEqual(t, initialHash, env.podTemplate().Annotations[ConfigHashAnnotationName])
	})

	t.Run("the certificate is removed with the last contributing Service when TLS is disabled for the API Service", func(t *testing.T) {
		env := newCertificateTestEnv(t)

		env.reconcile([]logstashv1alpha1.LogstashService{apiDisabled, beats(selfSignedTLS(false))})
		require.NotNil(t, env.certSecret())
		assert.True(t, hasHTTPCertsVolumeMount(env.podTemplate()))

		env.reconcile([]logstashv1alpha1.LogstashService{apiDisabled, beats(selfSignedTLS(true))})
		assert.Nil(t, env.certSecret())
		assert.False(t, hasHTTPCertsVolumeMount(env.podTemplate()))
	})
}
