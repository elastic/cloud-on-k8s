// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package nodespec

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/gkampitakis/go-snaps/snaps"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	policyv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/stackconfigpolicy/v1alpha1"
	commonannotation "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/hash"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/keystore"
	common "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/initcontainer"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

type esSampleBuilder struct {
	userConfig              map[string]interface{}
	esAdditionalAnnotations map[string]string
	keystoreResources       *keystore.Resources
	transportCertsDisabled  bool
	version                 string
}

func newEsSampleBuilder() *esSampleBuilder {
	return &esSampleBuilder{}
}

func (esb *esSampleBuilder) build() esv1.Elasticsearch {
	es := sampleES.DeepCopy()
	for k, v := range esb.esAdditionalAnnotations {
		es.Annotations[k] = v
	}
	if esb.userConfig != nil {
		es.Spec.NodeSets[0].Config = &commonv1.Config{Data: esb.userConfig}
	}
	if esb.version != "" {
		es.Spec.Version = esb.version
	}
	es.Spec.Transport.TLS.SelfSignedCertificates = &esv1.SelfSignedTransportCertificates{Disabled: esb.transportCertsDisabled}
	return *es
}

func (esb *esSampleBuilder) withVersion(version string) *esSampleBuilder {
	esb.version = version
	return esb
}

func (esb *esSampleBuilder) withUserConfig(userConfig map[string]interface{}) *esSampleBuilder {
	esb.userConfig = userConfig
	return esb
}

func (esb *esSampleBuilder) addEsAnnotations(esAnnotations map[string]string) *esSampleBuilder {
	esb.esAdditionalAnnotations = esAnnotations
	return esb
}

func (esb *esSampleBuilder) withKeystoreResources(keystoreResources *keystore.Resources) *esSampleBuilder {
	esb.keystoreResources = keystoreResources
	return esb
}

func (esb *esSampleBuilder) withTransportCertsDisabled(disabled bool) *esSampleBuilder {
	esb.transportCertsDisabled = disabled
	return esb
}

var sampleES = esv1.Elasticsearch{
	ObjectMeta: metav1.ObjectMeta{
		Namespace: "namespace",
		Name:      "name",
		Labels: map[string]string{
			"cluster-label-name": "cluster-label-value",
		},
		Annotations: map[string]string{
			"cluster-annotation-name": "cluster-annotation-value",
		},
	},
	Spec: esv1.ElasticsearchSpec{
		Version: "7.2.0",
		NodeSets: []esv1.NodeSet{
			{
				Name:  "nodeset-1",
				Count: 2,
				Config: &commonv1.Config{
					Data: map[string]interface{}{
						"node.attr.foo": "bar",
						"node.master":   "true",
						"node.data":     "false",
					},
				},
				PodTemplate: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"pod-template-label-name": "pod-template-label-value",
						},
						Annotations: map[string]string{
							"pod-template-annotation-name": "pod-template-annotation-value",
						},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name: "additional-container",
							},
							{
								Name: "elasticsearch",
								Env: []corev1.EnvVar{
									{
										Name:  "my-env",
										Value: "my-value",
									},
								},
							},
						},
						InitContainers: []corev1.Container{
							{
								Name: "additional-init-container",
							},
						},
					},
				},
				VolumeClaimTemplates: []corev1.PersistentVolumeClaim{},
			},
			{
				Name:  "nodeset-1",
				Count: 2,
			},
		},
	},
}

func TestBuildPodTemplateSpecWithDefaultSecurityContext(t *testing.T) {
	for _, tt := range []struct {
		name                string
		version             version.Version
		setDefaultFSGroup   bool
		userSecurityContext *corev1.PodSecurityContext
		wantSecurityContext *corev1.PodSecurityContext
	}{
		{
			name:                "pre-8.0, setting off, no user context",
			version:             version.MustParse("7.8.0"),
			setDefaultFSGroup:   false,
			userSecurityContext: nil,
			wantSecurityContext: nil,
		},
		{
			name:                "pre-8.0, setting off, user context",
			version:             version.MustParse("7.8.0"),
			setDefaultFSGroup:   false,
			userSecurityContext: &corev1.PodSecurityContext{FSGroup: ptr.To[int64](123)},
			wantSecurityContext: &corev1.PodSecurityContext{FSGroup: ptr.To[int64](123)},
		},
		{
			name:                "pre-8.0, setting on, no user context",
			version:             version.MustParse("7.8.0"),
			setDefaultFSGroup:   true,
			userSecurityContext: nil,
			wantSecurityContext: nil,
		},
		{
			name:                "pre-8.0, setting on, user context",
			version:             version.MustParse("7.8.0"),
			setDefaultFSGroup:   true,
			userSecurityContext: &corev1.PodSecurityContext{FSGroup: ptr.To[int64](123)},
			wantSecurityContext: &corev1.PodSecurityContext{FSGroup: ptr.To[int64](123)},
		},
		{
			name:                "8.0+, setting off, no user context",
			version:             version.MustParse("8.0.0"),
			setDefaultFSGroup:   false,
			userSecurityContext: nil,
			wantSecurityContext: nil,
		},
		{
			name:                "8.0+, setting off, user context",
			version:             version.MustParse("8.0.0"),
			setDefaultFSGroup:   false,
			userSecurityContext: &corev1.PodSecurityContext{FSGroup: ptr.To[int64](123)},
			wantSecurityContext: &corev1.PodSecurityContext{FSGroup: ptr.To[int64](123)},
		},
		{
			name:                "8.0+, setting on, no user context",
			version:             version.MustParse("8.0.0"),
			setDefaultFSGroup:   true,
			userSecurityContext: nil,
			wantSecurityContext: &corev1.PodSecurityContext{FSGroup: ptr.To[int64](1000)},
		},
		{
			name:                "8.0+, setting on, user context",
			version:             version.MustParse("8.0.0"),
			setDefaultFSGroup:   true,
			userSecurityContext: &corev1.PodSecurityContext{FSGroup: ptr.To[int64](123)},
			wantSecurityContext: &corev1.PodSecurityContext{FSGroup: ptr.To[int64](123)},
		},
		{
			name:                "8.0+, setting on, empty user context",
			version:             version.MustParse("8.0.0"),
			setDefaultFSGroup:   true,
			userSecurityContext: &corev1.PodSecurityContext{},
			wantSecurityContext: &corev1.PodSecurityContext{},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			es := newEsSampleBuilder().build()
			es.Spec.Version = tt.version.String()
			es.Spec.NodeSets[0].PodTemplate.Spec.SecurityContext = tt.userSecurityContext

			cfg, err := settings.NewMergedESConfig(es.Name, tt.version, corev1.IPv4Protocol, es.Spec.HTTP, *es.Spec.NodeSets[0].Config, nil)
			require.NoError(t, err)

			client := k8s.NewFakeClient(&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: es.Namespace, Name: esv1.ScriptsConfigMap(es.Name)}})
			actual, err := BuildPodTemplateSpec(context.Background(), client, es, es.Spec.NodeSets[0], cfg, nil, tt.setDefaultFSGroup, PolicyConfig{})
			require.NoError(t, err)
			require.Equal(t, tt.wantSecurityContext, actual.Spec.SecurityContext)
		})
	}
}

func TestBuildPodTemplateSpec(t *testing.T) {
	// 7.20 fixtures
	sampleES := newEsSampleBuilder().build()
	policyEsConfig := common.MustCanonicalConfig(map[string]interface{}{
		"logger.org.elasticsearch.discovery": "DEBUG",
	})
	secretMounts := []policyv1alpha1.SecretMount{{
		SecretName: "test-es-secretname",
		MountPath:  "/usr/test",
	}}
	elasticsearchConfigAndMountsHash := hash.HashObject([]interface{}{policyEsConfig, secretMounts})
	policyConfig := PolicyConfig{
		ElasticsearchConfig: policyEsConfig,
		AdditionalVolumes: []volume.VolumeLike{
			volume.NewSecretVolumeWithMountPath("test-es-secretname", "test-es-secretname", "/usr/test"),
		},
		PolicyAnnotations: map[string]string{
			commonannotation.ElasticsearchConfigAndSecretMountsHashAnnotation: elasticsearchConfigAndMountsHash,
		},
	}
	// shared fixture
	scriptsConfigMap := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: sampleES.Namespace, Name: esv1.ScriptsConfigMap(sampleES.Name)}}

	type args struct {
		client                    k8s.Client
		es                        esv1.Elasticsearch
		keystoreResources         *keystore.Resources
		setDefaultSecurityContext bool
		policyConfig              PolicyConfig
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "v7.20",
			args: args{
				client:                    k8s.NewFakeClient(scriptsConfigMap),
				es:                        sampleES,
				keystoreResources:         &keystore.Resources{},
				setDefaultSecurityContext: false,
				policyConfig:              policyConfig,
			},
			wantErr: false,
		},
		{
			name: "v8.14.0",
			args: args{
				client: k8s.NewFakeClient(scriptsConfigMap),
				es:     newEsSampleBuilder().withVersion("8.14.0").build(),
			},
			wantErr: false,
		},
		{
			name: "failing client",
			args: args{
				client: k8s.NewFailingClient(errors.New("should fail")),
				es:     newEsSampleBuilder().withVersion("8.14.0").build(),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			es := tt.args.es
			nodeSet := es.Spec.NodeSets[0]

			ver, err := version.Parse(es.Spec.Version)
			require.NoError(t, err)

			cfg, err := settings.NewMergedESConfig(es.Name, ver, corev1.IPv4Protocol, es.Spec.HTTP, *nodeSet.Config, tt.args.policyConfig.ElasticsearchConfig)
			require.NoError(t, err)

			actual, err := BuildPodTemplateSpec(context.Background(), tt.args.client, es, es.Spec.NodeSets[0], cfg, tt.args.keystoreResources, tt.args.setDefaultSecurityContext, tt.args.policyConfig)
			if (err != nil) != tt.wantErr {
				t.Errorf("BuildPodTemplateSpec wantErr %v got %v", tt.wantErr, err)
			}

			// render as JSON for easier diff debugging
			gotJSON, err := json.MarshalIndent(&actual, " ", " ")
			require.NoError(t, err)
			snaps.MatchJSON(t, gotJSON)
		})
	}
}

func Test_buildAnnotations(t *testing.T) {
	type args struct {
		cfg                    map[string]interface{}
		esAnnotations          map[string]string
		keystoreResources      *keystore.Resources
		scriptsContent         string
		policyAnnotations      map[string]string
		transportCertsDisabled bool
	}
	tests := []struct {
		name                string
		args                args
		expectedAnnotations map[string]string
		wantErr             bool
	}{
		{
			name: "Sample Elasticsearch resource",
			expectedAnnotations: map[string]string{
				"elasticsearch.k8s.elastic.co/config-hash": "3089472956",
			},
		},
		{
			name: "Updated configuration",
			args: args{
				cfg: map[string]interface{}{
					"node.attr.foo": "bar",
					"node.master":   "false",
					"node.data":     "true",
				},
			},
			expectedAnnotations: map[string]string{
				"elasticsearch.k8s.elastic.co/config-hash": "1067885408",
			},
		},
		{
			name: "Simple Elasticsearch resource, with downward node labels",
			args: args{
				esAnnotations: map[string]string{"eck.k8s.elastic.co/downward-node-labels": "topology.kubernetes.io/zone"},
			},
			expectedAnnotations: map[string]string{
				"elasticsearch.k8s.elastic.co/config-hash": "3370858576",
			},
		},
		{
			name: "Simple Elasticsearch resource, with other downward node labels",
			args: args{
				esAnnotations: map[string]string{"eck.k8s.elastic.co/downward-node-labels": "topology.kubernetes.io/zone,topology.kubernetes.io/region"},
			},
			expectedAnnotations: map[string]string{
				"elasticsearch.k8s.elastic.co/config-hash": "909392898",
			},
		},
		{
			name: "With keystore and scripts content",
			args: args{
				keystoreResources: &keystore.Resources{
					Hash: "42",
				},
				scriptsContent: "scripts content",
			},
			expectedAnnotations: map[string]string{
				"elasticsearch.k8s.elastic.co/config-hash": "4033676039",
			},
		},
		{
			name: "With another keystore version",
			args: args{
				keystoreResources: &keystore.Resources{
					Hash: "43",
				},
				scriptsContent: "scripts content",
			},
			expectedAnnotations: map[string]string{
				"elasticsearch.k8s.elastic.co/config-hash": "4016898420",
			},
		},
		{
			name: "With another script version",
			args: args{
				keystoreResources: &keystore.Resources{
					Hash: "42",
				},
				scriptsContent: "another scripts content",
			},
			expectedAnnotations: map[string]string{
				"elasticsearch.k8s.elastic.co/config-hash": "3413674748",
			},
		},
		{
			name: "With policy annotations",
			args: args{
				policyAnnotations: map[string]string{
					commonannotation.ElasticsearchConfigAndSecretMountsHashAnnotation: "testhash",
				},
			},
			expectedAnnotations: map[string]string{
				commonannotation.ElasticsearchConfigAndSecretMountsHashAnnotation: "testhash",
			},
		},
		{
			name: "with transport certs disabled",
			args: args{
				transportCertsDisabled: true,
			},
			expectedAnnotations: map[string]string{
				"elasticsearch.k8s.elastic.co/self-signed-transport-cert-disabled": "true",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			es := newEsSampleBuilder().
				withKeystoreResources(tt.args.keystoreResources).
				withUserConfig(tt.args.cfg).
				addEsAnnotations(tt.args.esAnnotations).
				withTransportCertsDisabled(tt.args.transportCertsDisabled).
				build()
			ver, err := version.Parse(sampleES.Spec.Version)
			require.NoError(t, err)
			cfg, err := settings.NewMergedESConfig(es.Name, ver, corev1.IPv4Protocol, es.Spec.HTTP, *es.Spec.NodeSets[0].Config, nil)
			require.NoError(t, err)
			got := buildAnnotations(es, cfg, tt.args.keystoreResources, tt.args.scriptsContent, tt.args.policyAnnotations)

			for expectedAnnotation, expectedValue := range tt.expectedAnnotations {
				actualValue, exists := got[expectedAnnotation]
				assert.True(t, exists, "expected annotation: %s", expectedAnnotation)
				assert.Equal(t, expectedValue, actualValue, "expected value for annotation %s: %s, got %s", expectedAnnotation, expectedValue, actualValue)
			}
		})
	}
}

func Test_getDefaultContainerPorts(t *testing.T) {
	tt := []struct {
		name string
		es   esv1.Elasticsearch
		want []corev1.ContainerPort
	}{
		{
			name: "https",
			es:   sampleES,
			want: []corev1.ContainerPort{
				{Name: "https", HostPort: 0, ContainerPort: 9200, Protocol: "TCP", HostIP: ""},
				{Name: "transport", HostPort: 0, ContainerPort: 9300, Protocol: "TCP", HostIP: ""},
			},
		},
		{
			name: "http",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					HTTP: commonv1.HTTPConfig{
						TLS: commonv1.TLSOptions{
							SelfSignedCertificate: &commonv1.SelfSignedCertificate{
								Disabled: true,
							},
						},
					},
				},
			},
			want: []corev1.ContainerPort{
				{Name: "http", HostPort: 0, ContainerPort: 9200, Protocol: "TCP", HostIP: ""},
				{Name: "transport", HostPort: 0, ContainerPort: 9300, Protocol: "TCP", HostIP: ""},
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, getDefaultContainerPorts(tc.es), tc.want)
		})
	}
}

func Test_enableLog4JFormatMsgNoLookups(t *testing.T) {
	tt := []struct {
		name                       string
		version                    string
		userEnv                    []corev1.EnvVar
		expectedEsJavaOptsEnvValue string
	}{
		{
			name:                       "before 7.2.0, JVM log4j2.formatMsgNoLookups parameter is set by default",
			version:                    "7.0.0",
			userEnv:                    []corev1.EnvVar{{Name: "YO", Value: "LO"}},
			expectedEsJavaOptsEnvValue: "-Dlog4j2.formatMsgNoLookups=true",
		},
		{
			name:                       "before 7.2.0, JVM log4j2.formatMsgNoLookups parameter is merged with user-provided JVM parameters",
			version:                    "7.1.0",
			userEnv:                    []corev1.EnvVar{{Name: "ES_JAVA_OPTS", Value: "-Xms=42000 -Xmx=42000"}},
			expectedEsJavaOptsEnvValue: "-Dlog4j2.formatMsgNoLookups=true -Xms=42000 -Xmx=42000",
		},
		{
			name:                       "before 7.2.0, JVM log4j2.formatMsgNoLookups user-provided parameter is not overridden by us",
			version:                    "7.1.0",
			userEnv:                    []corev1.EnvVar{{Name: "ES_JAVA_OPTS", Value: "-Xms=42000 -Dlog4j2.formatMsgNoLookups=false -Xmx=42000"}},
			expectedEsJavaOptsEnvValue: "-Xms=42000 -Dlog4j2.formatMsgNoLookups=false -Xmx=42000",
		},
		{
			name:                       "since 7.2.0, JVM log4j2.formatMsgNoLookups parameter is not set by default",
			version:                    "7.2.0",
			userEnv:                    []corev1.EnvVar{{Name: "ES_JAVA_OPTS", Value: "-Xms=42000 -Xmx=42000"}},
			expectedEsJavaOptsEnvValue: "-Xms=42000 -Xmx=42000",
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			sampleES := newEsSampleBuilder().build()
			// set version
			sampleES.Spec.Version = tc.version
			// set user env
			sampleES.Spec.NodeSets[0].PodTemplate.Spec.Containers[1].Env = tc.userEnv

			ver, err := version.Parse(sampleES.Spec.Version)
			require.NoError(t, err)
			cfg, err := settings.NewMergedESConfig(sampleES.Name, ver, corev1.IPv4Protocol, sampleES.Spec.HTTP, *sampleES.Spec.NodeSets[0].Config, nil)
			require.NoError(t, err)
			client := k8s.NewFakeClient(&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: sampleES.Namespace, Name: esv1.ScriptsConfigMap(sampleES.Name)}})
			actual, err := BuildPodTemplateSpec(context.Background(), client, sampleES, sampleES.Spec.NodeSets[0], cfg, nil, false, PolicyConfig{})
			require.NoError(t, err)

			env := actual.Spec.Containers[1].Env
			envMap := make(map[string]string)
			for _, e := range env {
				envMap[e.Name] = e.Value
			}
			assert.Equal(t, len(env), len(envMap))
			assert.Equal(t, tc.expectedEsJavaOptsEnvValue, envMap[settings.EnvEsJavaOpts])
		})
	}
}

func Test_getScriptsConfigMapContent(t *testing.T) {
	cm := &corev1.ConfigMap{
		Data: map[string]string{
			PreStopHookScriptConfigKey:             "value1#",
			initcontainer.PrepareFsScriptConfigKey: "value2#",
			LegacyReadinessProbeScriptConfigKey:    "value3#",
			initcontainer.SuspendScriptConfigKey:   "value4#",
			initcontainer.SuspendedHostsFile:       "value5#",
		},
	}
	assert.Equal(t, "value1#value2#value3#value4#", getScriptsConfigMapContent(cm))
}
