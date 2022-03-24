// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package agent

import (
	"fmt"
	"hash"
	"path"
	"sort"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	agentv1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/agent/v1alpha1"
	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	commonassociation "github.com/elastic/cloud-on-k8s/pkg/controller/common/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/container"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/maps"
)

const (
	CAFileName = "ca.crt"

	ContainerName = "agent"

	ConfigVolumeName = "config"
	ConfigMountPath  = "/etc"
	ConfigFileName   = "agent.yml"

	FleetCertsVolumeName = "fleet-certs"
	FleetCertsMountPath  = "/usr/share/fleet-server/config/http-certs"

	DataVolumeName            = "agent-data"
	DataMountHostPathTemplate = "/var/lib/%s/%s/agent-data"
	DataMountPath             = "/usr/share/data"

	// ConfigHashAnnotationName is an annotation used to store the Agent config hash.
	ConfigHashAnnotationName = "agent.k8s.elastic.co/config-hash"

	// VersionLabelName is a label used to track the version of a Agent Pod.
	VersionLabelName = "agent.k8s.elastic.co/version"

	// Below are the names of environment variables used to configure Elastic Agent to Kibana connection in Fleet mode.
	KibanaFleetHost     = "KIBANA_FLEET_HOST"
	KibanaFleetUsername = "KIBANA_FLEET_USERNAME"
	KibanaFleetPassword = "KIBANA_FLEET_PASSWORD" //nolint:gosec
	KibanaFleetSetup    = "KIBANA_FLEET_SETUP"
	KibanaFleetCA       = "KIBANA_FLEET_CA"

	// Below are the names of environment variables used to configure Elastic Agent to Fleet connection in Fleet mode.
	FleetEnroll = "FLEET_ENROLL"
	FleetCA     = "FLEET_CA"
	FleetURL    = "FLEET_URL"

	// Below are the names of environment variables used to configure Fleet Server and its connection to Elasticsearch
	// in Fleet mode.
	FleetServerEnable                = "FLEET_SERVER_ENABLE"
	FleetServerCert                  = "FLEET_SERVER_CERT"
	FleetServerCertKey               = "FLEET_SERVER_CERT_KEY"
	FleetServerElasticsearchHost     = "FLEET_SERVER_ELASTICSEARCH_HOST"
	FleetServerElasticsearchUsername = "FLEET_SERVER_ELASTICSEARCH_USERNAME"
	FleetServerElasticsearchPassword = "FLEET_SERVER_ELASTICSEARCH_PASSWORD" //nolint:gosec
	FleetServerElasticsearchCA       = "FLEET_SERVER_ELASTICSEARCH_CA"
	FleetServerServiceToken          = "FLEET_SERVER_SERVICE_TOKEN" //nolint:gosec

	ubiSharedCAPath    = "/etc/pki/ca-trust/source/anchors/"
	ubiUpdateCmd       = "/usr/bin/update-ca-trust"
	debianSharedCAPath = "/usr/local/share/ca-certificates/"
	debianUpdateCmd    = "/usr/sbin/update-ca-certificates"
)

var (
	defaultResources = corev1.ResourceRequirements{
		Limits: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceMemory: resource.MustParse("350Mi"),
			corev1.ResourceCPU:    resource.MustParse("200m"),
		},
		Requests: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceMemory: resource.MustParse("350Mi"),
			corev1.ResourceCPU:    resource.MustParse("200m"),
		},
	}

	// defaultFleetResources defines default resources to use in case fleet mode is enabled.
	// System+Kubernetes integrations takes Elastic Agent to 70%, Fleet Server to 60% memory
	// usage of the below as of 7.14.0.
	defaultFleetResources = corev1.ResourceRequirements{
		Limits: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceMemory: resource.MustParse("1Gi"),
			corev1.ResourceCPU:    resource.MustParse("200m"),
		},
		Requests: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceMemory: resource.MustParse("1Gi"),
			corev1.ResourceCPU:    resource.MustParse("200m"),
		},
	}

	secretEnvVarNames = map[string]struct{}{
		KibanaFleetUsername:              {},
		KibanaFleetPassword:              {},
		FleetServerElasticsearchUsername: {},
		FleetServerElasticsearchPassword: {},
	}
)

func buildPodTemplate(params Params, fleetCerts *certificates.CertificatesSecret, configHash hash.Hash32) (corev1.PodTemplateSpec, error) {
	defer tracing.Span(&params.Context)()
	spec := &params.Agent.Spec
	builder := defaults.NewPodTemplateBuilder(params.GetPodTemplate(), ContainerName)
	vols := []volume.VolumeLike{
		// volume with agent configuration file
		volume.NewSecretVolume(
			ConfigSecretName(params.Agent.Name),
			ConfigVolumeName,
			path.Join(ConfigMountPath, ConfigFileName),
			ConfigFileName,
			0440),
	}

	// fleet mode requires some special treatment
	if spec.FleetModeEnabled() {
		var err error
		if builder, err = amendBuilderForFleetMode(params, fleetCerts, builder, configHash); err != nil {
			return corev1.PodTemplateSpec{}, err
		}
	} else if spec.StandaloneModeEnabled() {
		// cleanup secret used in Fleet mode
		if err := cleanupEnvVarsSecret(params); err != nil {
			return corev1.PodTemplateSpec{}, err
		}

		builder = builder.
			WithResources(defaultResources).
			WithArgs("-e", "-c", path.Join(ConfigMountPath, ConfigFileName))

		// volume with agent data path
		vols = append(vols, createDataVolume(params))
	}

	// all volumes with CAs of direct associations
	caAssocVols, err := getVolumesFromAssociations(params.Agent.GetAssociations())
	if err != nil {
		return corev1.PodTemplateSpec{}, err
	}
	vols = append(vols, caAssocVols...)

	labels := maps.Merge(NewLabels(params.Agent), map[string]string{
		VersionLabelName: spec.Version})

	annotations := map[string]string{
		ConfigHashAnnotationName: fmt.Sprint(configHash.Sum32()),
	}

	builder = builder.
		WithLabels(labels).
		WithAnnotations(annotations).
		WithDockerImage(spec.Image, container.ImageRepository(container.AgentImage, spec.Version)).
		WithAutomountServiceAccountToken().
		WithVolumeLikes(vols...).
		WithEnv(
			corev1.EnvVar{Name: "NODE_NAME", ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "spec.nodeName",
				},
			}},
		)

	return builder.PodTemplate, nil
}

func amendBuilderForFleetMode(params Params, fleetCerts *certificates.CertificatesSecret, builder *defaults.PodTemplateBuilder, configHash hash.Hash) (*defaults.PodTemplateBuilder, error) {
	esAssociation, err := getRelatedEsAssoc(params)
	if err != nil {
		return nil, err
	}

	builder, err = applyRelatedEsAssoc(params.Agent, esAssociation, builder)
	if err != nil {
		return nil, err
	}

	err = writeEsAssocToConfigHash(params, esAssociation, configHash)
	if err != nil {
		return nil, err
	}

	builder, err = applyEnvVars(params, builder)
	if err != nil {
		return nil, err
	}

	if params.Agent.Spec.FleetServerEnabled {
		// ECK creates CA and a certificate for Fleet Server to use. This volume contains those.
		builder = builder.WithVolumeLikes(
			volume.NewSecretVolumeWithMountPath(
				fleetCerts.Name,
				FleetCertsVolumeName,
				FleetCertsMountPath,
			))

		builder = builder.WithPorts([]corev1.ContainerPort{{Name: params.Agent.Spec.HTTP.Protocol(), ContainerPort: FleetServerPort, Protocol: corev1.ProtocolTCP}})
	}

	builder = builder.
		WithResources(defaultFleetResources).
		// needed to pick up fleet-setup.yml correctly
		WithEnv(corev1.EnvVar{Name: "CONFIG_PATH", Value: "/usr/share/elastic-agent"})

	return builder, nil
}

func applyEnvVars(params Params, builder *defaults.PodTemplateBuilder) (*defaults.PodTemplateBuilder, error) {
	fleetModeEnvVars, err := getFleetModeEnvVars(params.Agent, params.Client)
	if err != nil {
		return nil, err
	}

	type tuple struct{ k, v string }
	sortedVars := make([]tuple, 0, len(fleetModeEnvVars))
	for k, v := range fleetModeEnvVars {
		sortedVars = append(sortedVars, tuple{k: k, v: v})
	}
	sort.Slice(sortedVars, func(i, j int) bool {
		return sortedVars[i].k < sortedVars[j].k
	})

	envVarsSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      EnvVarsSecretName(params.Agent.Name),
			Namespace: params.Agent.Namespace,
			Labels:    common.AddCredentialsLabel(NewLabels(params.Agent)),
		},
		Data: map[string][]byte{},
	}
	for _, kv := range sortedVars {
		k, v := kv.k, kv.v
		if _, ok := secretEnvVarNames[k]; !ok {
			builder = builder.WithEnv(corev1.EnvVar{Name: k, Value: v})
			continue
		}

		// Checking if we really provide an env var to the container or it's already specified by the user. This is done
		// to allow for a proper cleanup and to prevent abandoning working credentials (user/pass) in a Secret that is
		// not used by the container.
		var isNew bool
		if builder, isNew = builder.WithNewEnv(corev1.EnvVar{Name: k, ValueFrom: secretSource(params.Agent.Name, k)}); isNew {
			envVarsSecret.Data[k] = []byte(v)
		}
	}

	// cleanup and don't reconcile if there are no env vars provided from a secret
	if len(envVarsSecret.Data) == 0 {
		if err := cleanupEnvVarsSecret(params); err != nil {
			return nil, err
		}
	} else if _, err := reconciler.ReconcileSecret(params.Client, envVarsSecret, &params.Agent); err != nil {
		return nil, err
	}

	return builder, nil
}

func getRelatedEsAssoc(params Params) (commonv1.Association, error) {
	var esAssociation commonv1.Association
	//nolint:nestif
	if params.Agent.Spec.FleetServerEnabled {
		// As the reference chain is: Fleet Server ---> Elasticsearch,
		// we just grab the reference to Elasticsearch from the current agent (Fleet Server).
		var err error
		esAssociation, err = association.SingleAssociationOfType(params.Agent.GetAssociations(), commonv1.ElasticsearchAssociationType)
		if err != nil {
			return nil, err
		}
	} else {
		// As the reference chain is: Elastic Agent ---> Fleet Server ---> Elasticsearch,
		// we need first to identify the Fleet Server and then identify its reference to Elasticsearch.
		fs, err := getAssociatedFleetServer(params)
		if err != nil {
			return nil, err
		}

		if fs != nil {
			var err error
			esAssociation, err = association.SingleAssociationOfType(fs.GetAssociations(), commonv1.ElasticsearchAssociationType)
			if err != nil {
				return nil, err
			}
		}
	}
	return esAssociation, nil
}

func applyRelatedEsAssoc(agent agentv1alpha1.Agent, esAssociation commonv1.Association, builder *defaults.PodTemplateBuilder) (*defaults.PodTemplateBuilder, error) {
	if esAssociation == nil {
		return builder, nil
	}

	if !agent.Spec.FleetServerEnabled && agent.Namespace != esAssociation.AssociationRef().Namespace {
		return nil, fmt.Errorf(
			"agent namespace %s is different than referenced Elasticsearch namespace %s, this is not supported yet",
			agent.Namespace,
			esAssociation.AssociationRef().Namespace,
		)
	}

	assocConf, err := esAssociation.AssociationConf()
	if err != nil {
		return nil, err
	}
	builder = builder.WithVolumeLikes(volume.NewSecretVolumeWithMountPath(
		assocConf.GetCASecretName(),
		fmt.Sprintf("%s-certs", esAssociation.AssociationType()),
		certificatesDir(esAssociation),
	))

	// Beats managed by the Elastic Agent don't trust the Elasticsearch CA that Elastic Agent itself is configured
	// to trust. There is currently no way to configure those Beats to trust a particular CA. The intended way to handle
	// it is to allow Fleet to provide Beat output settings, but due to https://github.com/elastic/kibana/issues/102794
	// this is not supported outside of UI. To workaround this limitation the Agent is going to update Pod-wide CA store
	// before starting Elastic Agent.
	cmd := trustCAScript(path.Join(certificatesDir(esAssociation), CAFileName))
	return builder.WithCommand([]string{"/usr/bin/env", "bash", "-c", cmd}), nil
}

func writeEsAssocToConfigHash(params Params, esAssociation commonv1.Association, configHash hash.Hash) error {
	if esAssociation == nil || params.Agent.Spec.FleetServerEnabled {
		return nil
	}

	// Because of the reference chain (Elastic Agent ---> Fleet Server ---> Elasticsearch), we are going to get
	// notified when CA of Elasticsearch changes as Fleet Server resource will get updated as well. But what we
	// also need to do is to roll Elastic Agent Pods to pick up the update CA. To be able to do that, we are
	// adding Fleet Server associations (which includes Elasticsearch) to config hash attached to Elastic Agent
	// Pods.
	return commonassociation.WriteAssocsToConfigHash(
		params.Client,
		[]commonv1.Association{esAssociation},
		configHash,
	)
}

func getVolumesFromAssociations(associations []commonv1.Association) ([]volume.VolumeLike, error) {
	var vols []volume.VolumeLike //nolint:prealloc
	for i, assoc := range associations {
		assocConf, err := assoc.AssociationConf()
		if err != nil {
			return nil, err
		}
		if !assocConf.CAIsConfigured() {
			// skip as there is no volume to mount if association has no CA configured
			continue
		}
		caSecretName := assocConf.GetCASecretName()
		vols = append(vols, volume.NewSecretVolumeWithMountPath(
			caSecretName,
			fmt.Sprintf("%s-certs-%d", assoc.AssociationType(), i),
			certificatesDir(assoc),
		))
	}
	return vols, nil
}

func getAssociatedFleetServer(params Params) (commonv1.Associated, error) {
	assoc, err := association.SingleAssociationOfType(params.Agent.GetAssociations(), commonv1.FleetServerAssociationType)
	if err != nil {
		return nil, err
	}
	if assoc == nil {
		return nil, nil
	}

	fsRef := assoc.AssociationRef()
	fs := agentv1alpha1.Agent{}
	request := reconcile.Request{NamespacedName: fsRef.NamespacedName()}
	if err = params.Client.Get(params.Context, request.NamespacedName, &fs); err != nil {
		return nil, err
	}

	return &fs, nil
}

func trustCAScript(caPath string) string {
	return fmt.Sprintf(`#!/usr/bin/env bash
set -e
if [[ -f %[1]s ]]; then
  if [[ -f %[3]s ]]; then
    cp %[1]s %[2]s
    %[3]s
  elif [[ -f %[5]s ]]; then
    cp %[1]s %[4]s
    %[5]s
  fi
fi
/usr/bin/tini -- /usr/local/bin/docker-entrypoint -e
`, caPath, ubiSharedCAPath, ubiUpdateCmd, debianSharedCAPath, debianUpdateCmd)
}

func createDataVolume(params Params) volume.VolumeLike {
	dataMountHostPath := fmt.Sprintf(DataMountHostPathTemplate, params.Agent.Namespace, params.Agent.Name)

	return volume.NewHostVolume(
		DataVolumeName,
		dataMountHostPath,
		DataMountPath,
		false,
		corev1.HostPathDirectoryOrCreate)
}

func certificatesDir(association commonv1.Association) string {
	ref := association.AssociationRef()
	return fmt.Sprintf(
		"/mnt/elastic-internal/%s-association/%s/%s/certs",
		association.AssociationType(),
		ref.Namespace,
		ref.Name,
	)
}

func getFleetModeEnvVars(agent agentv1alpha1.Agent, client k8s.Client) (map[string]string, error) {
	result := map[string]string{}

	for _, f := range []func(agentv1alpha1.Agent, k8s.Client) (map[string]string, error){
		getFleetSetupKibanaEnvVars,
		getFleetSetupFleetEnvVars,
		getFleetSetupFleetServerEnvVars,
	} {
		envVars, err := f(agent, client)
		if err != nil {
			return nil, err
		}
		result = maps.Merge(result, envVars)
	}

	return result, nil
}

func getFleetSetupKibanaEnvVars(agent agentv1alpha1.Agent, client k8s.Client) (map[string]string, error) {
	if agent.Spec.KibanaRef.IsDefined() {
		kbConnectionSettings, err := extractConnectionSettings(agent, client, commonv1.KibanaAssociationType)
		if err != nil {
			return nil, err
		}

		envVars := map[string]string{
			KibanaFleetHost:     kbConnectionSettings.host,
			KibanaFleetUsername: kbConnectionSettings.credentials.Username,
			KibanaFleetPassword: kbConnectionSettings.credentials.Password,
			KibanaFleetSetup:    strconv.FormatBool(agent.Spec.KibanaRef.IsDefined()),
		}

		// don't set ca key if ca is not available
		if kbConnectionSettings.ca != "" {
			envVars[KibanaFleetCA] = kbConnectionSettings.ca
		}

		return envVars, nil
	}

	return map[string]string{}, nil
}

func getFleetSetupFleetEnvVars(agent agentv1alpha1.Agent, client k8s.Client) (map[string]string, error) {
	fleetCfg := map[string]string{}

	if agent.Spec.KibanaRef.IsDefined() {
		fleetCfg[FleetEnroll] = "true"
	}

	// Agent in Fleet mode can run as a Fleet Server or as an Elastic Agent that connects to Fleet Server.
	// Both cases are handled below and the presence of FleetServerRef indicates the latter case.
	if agent.Spec.FleetServerEnabled { //nolint:nestif
		fleetURL, err := association.ServiceURL(
			client,
			types.NamespacedName{Namespace: agent.Namespace, Name: HTTPServiceName(agent.Name)},
			agent.Spec.HTTP.Protocol(),
		)
		if err != nil {
			return nil, err
		}

		fleetCfg[FleetURL] = fleetURL
		fleetCfg[FleetCA] = path.Join(FleetCertsMountPath, certificates.CAFileName)
	} else if agent.Spec.FleetServerRef.IsDefined() {
		assoc, err := association.SingleAssociationOfType(agent.GetAssociations(), commonv1.FleetServerAssociationType)
		if err != nil {
			return nil, err
		}
		if assoc == nil {
			return fleetCfg, nil
		}
		assocConf, err := assoc.AssociationConf()
		if err != nil {
			return nil, err
		}
		fleetCfg[FleetURL] = assocConf.GetURL()
		if assocConf.GetCACertProvided() {
			fleetCfg[FleetCA] = path.Join(certificatesDir(assoc), CAFileName)
		}
	}

	return fleetCfg, nil
}

func getFleetSetupFleetServerEnvVars(agent agentv1alpha1.Agent, client k8s.Client) (map[string]string, error) {
	if !agent.Spec.FleetServerEnabled {
		return map[string]string{}, nil
	}

	fleetServerCfg := map[string]string{
		FleetServerEnable:  "true",
		FleetServerCert:    path.Join(FleetCertsMountPath, certificates.CertFileName),
		FleetServerCertKey: path.Join(FleetCertsMountPath, certificates.KeyFileName),
	}

	esExpected := len(agent.Spec.ElasticsearchRefs) > 0 && agent.Spec.ElasticsearchRefs[0].IsDefined()
	if esExpected {
		esConnectionSettings, err := extractConnectionSettings(agent, client, commonv1.ElasticsearchAssociationType)
		if err != nil {
			return nil, err
		}

		fleetServerCfg[FleetServerElasticsearchHost] = esConnectionSettings.host

		if esConnectionSettings.credentials.HasServiceAccountToken() {
			fleetServerCfg[FleetServerServiceToken] = esConnectionSettings.credentials.ServiceAccountToken
		} else {
			fleetServerCfg[FleetServerElasticsearchUsername] = esConnectionSettings.credentials.Username
			fleetServerCfg[FleetServerElasticsearchPassword] = esConnectionSettings.credentials.Password
		}

		// don't set ca key if ca is not available
		if esConnectionSettings.ca != "" {
			fleetServerCfg[FleetServerElasticsearchCA] = esConnectionSettings.ca
		}
	}

	return fleetServerCfg, nil
}

func secretSource(name, key string) *corev1.EnvVarSource {
	f := false
	return &corev1.EnvVarSource{
		SecretKeyRef: &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: EnvVarsSecretName(name),
			},
			Key:      key,
			Optional: &f,
		},
	}
}

func cleanupEnvVarsSecret(params Params) error {
	var envVarsSecret corev1.Secret
	if err := params.Client.Get(
		params.Context,
		types.NamespacedName{Name: EnvVarsSecretName(params.Agent.Name), Namespace: params.Agent.Namespace},
		&envVarsSecret,
	); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
	} else if err := params.Client.Delete(params.Context, &envVarsSecret); err != nil {
		return err
	}

	return nil
}
