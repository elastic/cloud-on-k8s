// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package agent

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	toolsevents "k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	agentv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/agent/v1alpha1"
	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/certificates"
	commonlicense "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/scheme"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

func Test_isFleetServerClientAuthRequired(t *testing.T) {
	tests := []struct {
		name            string
		agentVersion    string // defaults to 9.5.0 if empty
		specClientAuth  bool
		tlsDisabled     bool
		envVar          *corev1.EnvVar
		enterpriseOff   bool
		nilChecker      bool
		wantRequired    bool
		wantWarning     bool
		wantWarningLike string
	}{
		{
			name:           "spec true, no env, TLS on: required",
			specClientAuth: true,
			wantRequired:   true,
		},
		{
			name:           "spec false, no env, TLS on: not required",
			specClientAuth: false,
			wantRequired:   false,
		},
		{
			name:           "TLS disabled: not required, no warning",
			specClientAuth: false,
			tlsDisabled:    true,
			wantRequired:   false,
		},
		{
			name:           "spec true, env=required, TLS on: required, no warning",
			specClientAuth: true,
			envVar:         &corev1.EnvVar{Name: FleetServerClientAuth, Value: FleetServerClientAuthRequired},
			wantRequired:   true,
		},
		{
			name:           "spec false, env=required, TLS on: required, no warning",
			specClientAuth: false,
			envVar:         &corev1.EnvVar{Name: FleetServerClientAuth, Value: FleetServerClientAuthRequired},
			wantRequired:   true,
		},
		{
			name:            "spec true, env=optional, TLS on: not required, warning",
			specClientAuth:  true,
			envVar:          &corev1.EnvVar{Name: FleetServerClientAuth, Value: "optional"},
			wantRequired:    false,
			wantWarning:     true,
			wantWarningLike: `FLEET_SERVER_CLIENT_AUTH is set to "optional"`,
		},
		{
			name:           "spec false, env=optional, TLS on: not required, no warning",
			specClientAuth: false,
			envVar:         &corev1.EnvVar{Name: FleetServerClientAuth, Value: "optional"},
			wantRequired:   false,
		},
		{
			name:            "spec true, env=none, TLS on: not required, warning",
			specClientAuth:  true,
			envVar:          &corev1.EnvVar{Name: FleetServerClientAuth, Value: "none"},
			wantRequired:    false,
			wantWarning:     true,
			wantWarningLike: `FLEET_SERVER_CLIENT_AUTH is set to "none"`,
		},
		{
			name:           "spec false, env=none, TLS on: not required, no warning",
			specClientAuth: false,
			envVar:         &corev1.EnvVar{Name: FleetServerClientAuth, Value: "none"},
			wantRequired:   false,
		},
		{
			name:           "spec false, env=required, TLS disabled: not required, no warning",
			specClientAuth: false,
			tlsDisabled:    true,
			envVar:         &corev1.EnvVar{Name: FleetServerClientAuth, Value: FleetServerClientAuthRequired},
			wantRequired:   false,
		},
		{
			name:            "no enterprise license: not required, warning",
			specClientAuth:  true,
			enterpriseOff:   true,
			wantRequired:    false,
			wantWarning:     true,
			wantWarningLike: "Enterprise license is not enabled",
		},
		{
			name:           "nil license checker: not required",
			specClientAuth: true,
			nilChecker:     true,
			wantRequired:   false,
		},
		{
			// Validation blocks spec.http.tls.client.authentication=true for unsupported versions,
			// but a manually-set FLEET_SERVER_CLIENT_AUTH env var must be honored regardless.
			name:         "unsupported version, FLEET_SERVER_CLIENT_AUTH=required set manually: required",
			agentVersion: "8.15.0",
			envVar:       &corev1.EnvVar{Name: FleetServerClientAuth, Value: FleetServerClientAuthRequired},
			wantRequired: true,
		},
		{
			name:           "spec true, env via valueFrom, TLS on: not required, warning",
			specClientAuth: true,
			envVar: &corev1.EnvVar{
				Name: FleetServerClientAuth,
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						Name: "my-secret",
						Key:  "client-auth",
					},
				},
			},
			wantRequired:    false,
			wantWarning:     true,
			wantWarningLike: "valueFrom",
		},
		{
			name:           "spec false, env via valueFrom, TLS on: not required, warning",
			specClientAuth: false,
			envVar: &corev1.EnvVar{
				Name: FleetServerClientAuth,
				ValueFrom: &corev1.EnvVarSource{
					ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
						Name: "my-configmap",
						Key:  "client-auth",
					},
				},
			},
			wantRequired:    false,
			wantWarning:     true,
			wantWarningLike: "valueFrom",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent := agentv1alpha1.Agent{
				Spec: agentv1alpha1.AgentSpec{
					Deployment: &agentv1alpha1.DeploymentSpec{},
				},
			}
			agent.Spec.HTTP.TLS.Client.Authentication = tt.specClientAuth
			if tt.tlsDisabled {
				agent.Spec.HTTP.TLS.SelfSignedCertificate = &commonv1.SelfSignedCertificate{Disabled: true}
			}
			if tt.envVar != nil {
				agent.Spec.Deployment.PodTemplate.Spec.Containers = []corev1.Container{
					{Name: "agent", Env: []corev1.EnvVar{*tt.envVar}},
				}
			}

			agentVersion := tt.agentVersion
			if agentVersion == "" {
				agentVersion = "9.5.0"
			}
			params := Params{
				Context:      context.Background(),
				Agent:        agent,
				AgentVersion: version.MustParse(agentVersion),
			}
			if !tt.nilChecker {
				params.LicenseChecker = commonlicense.MockLicenseChecker{EnterpriseEnabled: !tt.enterpriseOff}
			}

			gotRequired, gotWarning, err := isFleetServerClientAuthRequired(params)
			require.NoError(t, err)
			require.Equal(t, tt.wantRequired, gotRequired)
			require.Equal(t, tt.wantWarning, gotWarning != "", "unexpected warning presence: %q", gotWarning)
			if tt.wantWarningLike != "" {
				require.Contains(t, gotWarning, tt.wantWarningLike)
			}
		})
	}
}

func Test_reconcileFleetServerClientAuth(t *testing.T) {
	const (
		agentName      = "test-agent"
		agentNamespace = "test-ns"
	)
	certRotation := certificates.RotationParams{
		Validity:     24 * time.Hour,
		RotateBefore: 1 * time.Hour,
	}
	operatorClientCertSecretName := certificates.OperatorClientCertSecretName(Namer, agentName)
	trustBundleSecretName := certificates.ClientCertTrustBundleSecretName(Namer, agentName)

	newAgent := func(annotations map[string]string) agentv1alpha1.Agent {
		return agentv1alpha1.Agent{
			Name:        agentName,
			Namespace:   agentNamespace,
			Annotations: annotations,
			Spec: agentv1alpha1.AgentSpec{
				FleetServerEnabled: true,
				Deployment:         &agentv1alpha1.DeploymentSpec{},
			},
		}
	}

	clientCA, err := certificates.NewSelfSignedCA(certificates.CABuilderOptions{})
	require.NoError(t, err)
	clientCAPEM := certificates.EncodePEMCert(clientCA.Cert.Raw)
	clientCAConfigMap := corev1.ConfigMap{
		Name: "client-cas", Namespace: agentNamespace,
		Data: map[string]string{certificates.CAFileName: string(clientCAPEM)},
	}
	clientCAsWatchKey := certificates.ClientCAWatchKey(Namer, types.NamespacedName{Namespace: agentNamespace, Name: agentName})
	withClientCAs := func(agent agentv1alpha1.Agent) agentv1alpha1.Agent {
		agent.Spec.HTTP.TLS.Client.CertificateAuthorities = commonv1.ConfigMapRef{ConfigMapName: clientCAConfigMap.Name}
		return agent
	}

	tests := []struct {
		name               string
		agent              agentv1alpha1.Agent
		existingSecrets    []corev1.Secret
		clientAuthRequired bool
		fleetCerts         *certificates.CertificatesSecret
		wantAnnotation     bool
		wantOperatorCert   bool
		wantTrustBundle    bool
		wantTrustBundleCA  bool
		// existingCAConfigMap creates the config map of the client CAs before reconciling.
		existingCAConfigMap bool
		// existingWatch registers a watch on the client CAs before reconciling.
		existingWatch bool
		// unreadable makes the client CAs config map unreadable with an error other than NotFound.
		unreadable         bool
		wantClientCAs      bool
		wantClientCAsWatch bool
		wantErr            bool
		// wantClientCAsErr is the error returned apart from the results, not to hold back the Pods.
		wantClientCAsErr bool
	}{
		{
			name:               "clientAuthRequired=false, no annotation: no-op",
			agent:              newAgent(nil),
			clientAuthRequired: false,
			wantAnnotation:     false,
			wantOperatorCert:   false,
			wantTrustBundle:    false,
		},
		{
			name: "clientAuthRequired=false, annotation present: cleanup",
			agent: newAgent(map[string]string{
				annotation.ClientAuthenticationRequiredAnnotation: "true",
			}),
			existingSecrets: []corev1.Secret{
				{Name: operatorClientCertSecretName, Namespace: agentNamespace},
				{Name: trustBundleSecretName, Namespace: agentNamespace},
			},
			clientAuthRequired: false,
			wantAnnotation:     false,
			wantOperatorCert:   false,
			wantTrustBundle:    false,
		},
		{
			name:               "clientAuthRequired=true, no fleet certs: creates operator cert and trust bundle",
			agent:              newAgent(nil),
			clientAuthRequired: true,
			fleetCerts:         nil,
			wantAnnotation:     true,
			wantOperatorCert:   true,
			wantTrustBundle:    true,
			wantTrustBundleCA:  false,
		},
		{
			name:               "clientAuthRequired=true, fleet certs with cert: trust bundle includes HTTP cert",
			agent:              newAgent(nil),
			clientAuthRequired: true,
			fleetCerts: &certificates.CertificatesSecret{
				Data: map[string][]byte{
					certificates.CertFileName: []byte("fleet-server-cert-data"),
				},
			},
			wantAnnotation:    true,
			wantOperatorCert:  true,
			wantTrustBundle:   true,
			wantTrustBundleCA: true,
		},
		{
			name:                "clientAuthRequired=true, client CAs: trust bundle includes them and their config map is watched",
			agent:               withClientCAs(newAgent(nil)),
			existingCAConfigMap: true,
			clientAuthRequired:  true,
			wantAnnotation:      true,
			wantOperatorCert:    true,
			wantTrustBundle:     true,
			wantClientCAs:       true,
			wantClientCAsWatch:  true,
		},
		{
			name:               "clientAuthRequired=true, client CAs config map missing: error apart from the results, trust bundle without them, the config map is watched",
			agent:              withClientCAs(newAgent(nil)),
			clientAuthRequired: true,
			wantClientCAsErr:   true,
			wantAnnotation:     true,
			wantOperatorCert:   true,
			wantTrustBundle:    true,
			wantClientCAsWatch: true,
		},
		{
			name:                "clientAuthRequired=true, client CAs config map unreadable: error in the results, no trust bundle, the config map is watched",
			agent:               withClientCAs(newAgent(nil)),
			existingCAConfigMap: true,
			unreadable:          true,
			clientAuthRequired:  true,
			wantErr:             true,
			wantAnnotation:      true,
			wantOperatorCert:    true,
			wantTrustBundle:     false,
			wantClientCAsWatch:  true,
		},
		{
			name:                "clientAuthRequired=true, client CAs removed: watch is removed",
			agent:               newAgent(nil),
			existingCAConfigMap: true,
			existingWatch:       true,
			clientAuthRequired:  true,
			wantAnnotation:      true,
			wantOperatorCert:    true,
			wantTrustBundle:     true,
		},
		{
			name: "clientAuthRequired=false, annotation present, client CAs: cleanup, no warning, watch is kept until the annotation is removed",
			agent: withClientCAs(newAgent(map[string]string{
				annotation.ClientAuthenticationRequiredAnnotation: "true",
			})),
			existingCAConfigMap: true,
			existingSecrets: []corev1.Secret{
				{Name: operatorClientCertSecretName, Namespace: agentNamespace},
				{Name: trustBundleSecretName, Namespace: agentNamespace},
			},
			existingWatch:      true,
			clientAuthRequired: false,
			wantClientCAsWatch: true,
		},
		{
			name:                "clientAuthRequired=false, no annotation, client CAs: warning, watch is removed",
			agent:               withClientCAs(newAgent(nil)),
			existingCAConfigMap: true,
			existingWatch:       true,
			clientAuthRequired:  false,
		},
		{
			name:               "clientAuthRequired=false, no annotation, stale watch: watch is removed",
			agent:              newAgent(nil),
			existingWatch:      true,
			clientAuthRequired: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build the initial set of objects for the fake client.
			initObjs := slices.Clone(tt.existingSecrets)

			agent := tt.agent.DeepCopy()
			fakeClient := k8s.NewFakeClientBuilder(agent).WithInterceptorFuncs(interceptor.Funcs{
				Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
					if tt.unreadable && key.Name == clientCAConfigMap.Name {
						return apierrors.NewInternalError(fmt.Errorf("cannot read %s", key))
					}
					return c.Get(ctx, key, obj, opts...)
				},
			}).Build()
			// Add any pre-existing secrets.
			for i := range initObjs {
				require.NoError(t, fakeClient.Create(context.Background(), &initObjs[i]))
			}
			if tt.existingCAConfigMap {
				require.NoError(t, fakeClient.Create(context.Background(), clientCAConfigMap.DeepCopy()))
			}

			dynamicWatches := watches.NewDynamicWatches("testCtr")
			if tt.existingWatch {
				require.NoError(t, dynamicWatches.ConfigMaps.AddHandler(watches.NamedWatch[*corev1.ConfigMap]{
					Name:    clientCAsWatchKey,
					Watched: []types.NamespacedName{{Namespace: agentNamespace, Name: clientCAConfigMap.Name}},
					Watcher: k8s.ExtractNamespacedName(agent),
				}))
			}
			recorder := toolsevents.NewFakeRecorder(10)
			params := Params{
				Context:       context.Background(),
				Client:        fakeClient,
				EventRecorder: recorder,
				Watches:       dynamicWatches,
				Agent:         *agent,
				OperatorParams: operator.Parameters{
					CertRotation: certRotation,
				},
			}

			configHash := fnv.New32a()
			results, clientCAsErr := reconcileFleetServerClientAuth(params, tt.clientAuthRequired, tt.fleetCerts, configHash)
			require.Equal(t, tt.wantErr, results.HasError(), "unexpected error status in results")
			require.Equal(t, tt.wantClientCAsErr, clientCAsErr != nil, "unexpected client CAs error")
			// the trust bundle is written to the config hash, to roll the Pods, if and only if it is reconciled
			require.Equal(t, tt.wantTrustBundle, configHash.Sum32() != fnv.New32a().Sum32(), "unexpected config hash")
			if tt.wantClientCAsWatch {
				require.Contains(t, dynamicWatches.ConfigMaps.Registrations(), clientCAsWatchKey)
			} else {
				require.NotContains(t, dynamicWatches.ConfigMaps.Registrations(), clientCAsWatchKey)
			}
			// a warning is emitted if the client CAs are set but client authentication is neither required nor being disabled
			clientAuthInUse := tt.clientAuthRequired || annotation.HasClientAuthenticationRequired(agent)
			wantWarning := !clientAuthInUse && agent.Spec.HTTP.TLS.Client.CertificateAuthorities.IsDefined()
			require.Equal(t, wantWarning, len(recorder.Events) == 1)

			// Check annotation.
			var updatedAgent agentv1alpha1.Agent
			require.NoError(t, fakeClient.Get(context.Background(), types.NamespacedName{
				Namespace: agentNamespace,
				Name:      agentName,
			}, &updatedAgent))
			require.Equal(t, tt.wantAnnotation, annotation.HasClientAuthenticationRequired(&updatedAgent))

			// Check operator client cert secret.
			var operatorCertSecret corev1.Secret
			err := fakeClient.Get(context.Background(), types.NamespacedName{
				Namespace: agentNamespace,
				Name:      operatorClientCertSecretName,
			}, &operatorCertSecret)
			if tt.wantOperatorCert {
				require.NoError(t, err, "operator client cert secret should exist")
				require.NotEmpty(t, operatorCertSecret.Data[certificates.CertFileName], "operator client cert should have cert data")
			} else {
				require.True(t, apierrors.IsNotFound(err), "operator client cert secret should not exist")
			}

			// Check trust bundle secret.
			var trustBundleSecret corev1.Secret
			err = fakeClient.Get(context.Background(), types.NamespacedName{
				Namespace: agentNamespace,
				Name:      trustBundleSecretName,
			}, &trustBundleSecret)
			if tt.wantTrustBundle {
				require.NoError(t, err, "trust bundle secret should exist")
				bundleData := trustBundleSecret.Data[certificates.ClientCertificatesTrustBundleFileName]
				require.NotEmpty(t, bundleData, "trust bundle should have data")
				// When fleet certs have a cert, the trust bundle should include it.
				if tt.wantTrustBundleCA {
					require.Contains(t, string(bundleData), "fleet-server-cert-data", "trust bundle should contain the fleet server cert data")
				}
				if tt.wantClientCAs {
					require.Contains(t, string(bundleData), string(clientCAPEM), "trust bundle should contain the client CA")
				} else {
					require.NotContains(t, string(bundleData), string(clientCAPEM), "trust bundle should not contain the client CA")
				}
			} else {
				require.True(t, apierrors.IsNotFound(err), "trust bundle secret should not exist")
			}
		})
	}
}

func Test_internalReconcile_clientAuth(t *testing.T) {
	scheme.SetupScheme()
	certRotation := certificates.RotationParams{Validity: 24 * time.Hour, RotateBefore: time.Hour}

	readyDeployment := func(version string) []client.Object {
		return []client.Object{
			&appsv1.Deployment{
				Name: "fleet-server-agent", Namespace: "test",
				Status: appsv1.DeploymentStatus{Replicas: 1, ReadyReplicas: 1},
			},
			&corev1.Pod{
				Name:      "fleet-server-pod",
				Namespace: "test",
				Labels:    map[string]string{NameLabelName: "fleet-server", VersionLabelName: version},
				Status:    corev1.PodStatus{Phase: corev1.PodRunning},
			},
		}
	}

	tests := []struct {
		name         string
		agentVersion string
		// notFleetServer reconciles a former Fleet Server, now a standalone Agent without client CAs
		notFleetServer bool
		clientAuth     bool
		// missingClientCAs references a client CAs config map which does not exist
		missingClientCAs bool
		// staleClientCAsWatch registers the client CAs watch of a previous reconciliation
		staleClientCAsWatch bool
		// configUnwritable makes the reconciliation return before the Pods are reconciled
		configUnwritable bool
		extraObjs        []client.Object
		wantWarning      bool
		// wantEvents are expected in the warning events
		wantEvents []string
		wantHealth agentv1alpha1.AgentHealth
		wantErr    bool
		// wantErrMessages are expected in the error of the results
		wantErrMessages    []string
		wantPodsReconciled bool
		wantClientCAsWatch bool
	}{
		{
			name:         "client auth required, version not supported",
			agentVersion: "8.12.0",
			clientAuth:   true,
			wantWarning:  true,
			wantHealth:   agentv1alpha1.AgentRedHealth,
		},
		{
			name:                "client auth required, version not supported: the client CAs watch is kept until it is",
			agentVersion:        "8.12.0",
			clientAuth:          true,
			missingClientCAs:    true,
			staleClientCAsWatch: true,
			wantWarning:         true,
			wantHealth:          agentv1alpha1.AgentRedHealth,
			wantClientCAsWatch:  true,
		},
		{
			name:               "client auth required, exact minimum supported version",
			agentVersion:       "8.13.0",
			clientAuth:         true,
			extraObjs:          readyDeployment("8.13.0"),
			wantHealth:         agentv1alpha1.AgentGreenHealth,
			wantPodsReconciled: true,
		},
		{
			name:               "client auth not required, version not supported",
			agentVersion:       "8.12.0",
			clientAuth:         false,
			extraObjs:          readyDeployment("8.12.0"),
			wantHealth:         agentv1alpha1.AgentGreenHealth,
			wantPodsReconciled: true,
		},
		{
			name:                "not a Fleet Server anymore: the client CAs are not watched anymore",
			agentVersion:        "9.5.0",
			notFleetServer:      true,
			staleClientCAsWatch: true,
			extraObjs:           readyDeployment("9.5.0"),
			wantHealth:          agentv1alpha1.AgentGreenHealth,
			wantPodsReconciled:  true,
		},
		{
			name:               "client auth required, client CAs config map missing: error, the Pods are reconciled anyway",
			agentVersion:       "9.5.0",
			clientAuth:         true,
			missingClientCAs:   true,
			extraObjs:          readyDeployment("9.5.0"),
			wantWarning:        true,
			wantEvents:         []string{"Client certificate authorities reconciliation error: could not retrieve config map test/client-cas"},
			wantHealth:         agentv1alpha1.AgentGreenHealth,
			wantErr:            true,
			wantErrMessages:    []string{"could not retrieve config map test/client-cas"},
			wantPodsReconciled: true,
			wantClientCAsWatch: true,
		},
		{
			name:               "client auth required, client CAs config map missing and an error before the Pods are reconciled: both errors are reported",
			agentVersion:       "9.5.0",
			clientAuth:         true,
			missingClientCAs:   true,
			configUnwritable:   true,
			extraObjs:          readyDeployment("9.5.0"),
			wantWarning:        true,
			wantEvents:         []string{"Client certificate authorities reconciliation error: could not retrieve config map test/client-cas"},
			wantErr:            true,
			wantErrMessages:    []string{"could not retrieve config map test/client-cas", "cannot write the config"},
			wantClientCAsWatch: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agentObj := &agentv1alpha1.Agent{
				Name: "fleet-server", Namespace: "test",
				Spec: agentv1alpha1.AgentSpec{
					Version:            tt.agentVersion,
					FleetServerEnabled: true,
					Mode:               agentv1alpha1.AgentFleetMode,
					Deployment:         &agentv1alpha1.DeploymentSpec{},
				},
			}
			agentObj.Spec.HTTP.TLS.Client.Authentication = tt.clientAuth
			dynamicWatches := watches.NewDynamicWatches("testCtr")
			clientCAsWatchKey := certificates.ClientCAWatchKey(Namer, k8s.ExtractNamespacedName(agentObj))
			if tt.notFleetServer {
				agentObj.Spec.FleetServerEnabled = false
				agentObj.Spec.Mode = agentv1alpha1.AgentStandaloneMode
			}
			if tt.missingClientCAs {
				agentObj.Spec.HTTP.TLS.Client.CertificateAuthorities = commonv1.ConfigMapRef{ConfigMapName: "client-cas"}
			}
			if tt.staleClientCAsWatch {
				require.NoError(t, dynamicWatches.ConfigMaps.AddHandler(watches.NamedWatch[*corev1.ConfigMap]{
					Name:    clientCAsWatchKey,
					Watched: []types.NamespacedName{{Namespace: agentObj.Namespace, Name: "client-cas"}},
					Watcher: k8s.ExtractNamespacedName(agentObj),
				}))
			}

			agentVersion := version.MustParse(tt.agentVersion)
			recorder := toolsevents.NewFakeRecorder(10)
			objects := make([]client.Object, 1, 1+len(tt.extraObjs))
			objects[0] = agentObj
			objects = append(objects, tt.extraObjs...)
			fakeClient := k8s.NewFakeClientBuilder(objects...).WithInterceptorFuncs(interceptor.Funcs{
				Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
					if tt.configUnwritable && obj.GetName() == ConfigSecretName(agentObj.Name) {
						return apierrors.NewInternalError(errors.New("cannot write the config"))
					}
					return c.Create(ctx, obj, opts...)
				},
			}).Build()

			results, status := internalReconcile(Params{
				Context:        t.Context(),
				Client:         fakeClient,
				Watches:        dynamicWatches,
				EventRecorder:  recorder,
				Agent:          *agentObj,
				AgentVersion:   agentVersion,
				Status:         agentv1alpha1.AgentStatus{},
				LicenseChecker: commonlicense.MockLicenseChecker{EnterpriseEnabled: true},
				OperatorParams: operator.Parameters{
					CACertRotation: certRotation,
					CertRotation:   certRotation,
				},
			})

			close(recorder.Events)
			var warnings []string
			for e := range recorder.Events {
				if e != "" {
					warnings = append(warnings, e)
				}
			}

			assert.Equal(t, tt.wantHealth, status.Health, "unexpected health status")
			assert.Equal(t, tt.wantWarning, len(warnings) > 0, "unexpected warning event presence")
			for _, want := range tt.wantEvents {
				assert.True(t, slices.ContainsFunc(warnings, func(e string) bool { return strings.Contains(e, want) }),
					"expected a warning event containing %q, got %v", want, warnings)
			}
			assert.Equal(t, tt.wantErr, results.HasError(), "unexpected error status in results")
			_, err := results.Aggregate()
			for _, msg := range tt.wantErrMessages {
				assert.ErrorContains(t, err, msg)
			}
			assert.Equal(t, tt.wantClientCAsWatch, slices.Contains(dynamicWatches.ConfigMaps.Registrations(), clientCAsWatchKey),
				"unexpected client CAs watch")
			// the Deployment is created empty, its Pod template is set if the Pods are reconciled
			var deployment appsv1.Deployment
			err = fakeClient.Get(t.Context(), types.NamespacedName{Namespace: "test", Name: "fleet-server-agent"}, &deployment)
			assert.Equal(t, tt.wantPodsReconciled, err == nil && len(deployment.Spec.Template.Spec.Containers) > 0, "unexpected Pods reconciliation")
		})
	}
}
