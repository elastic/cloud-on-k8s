// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package agent

import (
	"context"
	"errors"
	"fmt"
	"hash"
	"path"
	"sort"
	"strings"

	pkgerrors "github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	agentv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/agent/v1alpha1"
	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/association"
	commonassociation "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/association"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/container"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/labels"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/maps"
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
	DataMountHostPathTemplate = "/var/lib/elastic-agent/%s/%s/state"
	DataMountPath             = "/usr/share/elastic-agent/state" // available since 7.13 functional since 7.15 without effect before that

	// ConfigHashAnnotationName is an annotation used to store the Agent config hash.
	ConfigHashAnnotationName = "agent.k8s.elastic.co/config-hash"

	// VersionLabelName is a label used to track the version of a Agent Pod.
	VersionLabelName = "agent.k8s.elastic.co/version"

	// Below are the names of environment variables used to configure Elastic Agent to Fleet connection in Fleet mode.
	FleetEnroll             = "FLEET_ENROLL"
	FleetEnrollmentToken    = "FLEET_ENROLLMENT_TOKEN"
	FleetCA                 = "FLEET_CA"
	FleetURL                = "FLEET_URL"
	FleetInsecure           = "FLEET_INSECURE"
	FleetServerInsecureHTTP = "FLEET_SERVER_INSECURE_HTTP"
	// FleetServerHost is the environment variable defining the binding host for Fleet Server HTTP.
	FleetServerHost = "FLEET_SERVER_HOST"
	// FleetServerPortEnv is the environment variable defining the binding port for Fleet Server HTTP.
	// *note* that the trailing 'Env' is required as 'FleetServerPort' is previously declared as int32.
	FleetServerPortEnv = "FLEET_SERVER_PORT"

	// Below are the names of environment variables used to configure Fleet Server and its connection to Elasticsearch
	// in Fleet mode.
	FleetServerEnable                = "FLEET_SERVER_ENABLE"
	FleetServerCert                  = "FLEET_SERVER_CERT"
	FleetServerCertKey               = "FLEET_SERVER_CERT_KEY"
	FleetServerElasticsearchHost     = "FLEET_SERVER_ELASTICSEARCH_HOST"
	FleetServerElasticsearchUsername = "FLEET_SERVER_ELASTICSEARCH_USERNAME"
	FleetServerElasticsearchPassword = "FLEET_SERVER_ELASTICSEARCH_PASSWORD" //nolint:gosec
	FleetServerElasticsearchCA       = "FLEET_SERVER_ELASTICSEARCH_CA"
	FleetServerPolicyID              = "FLEET_SERVER_POLICY_ID"
	FleetServerServiceToken          = "FLEET_SERVER_SERVICE_TOKEN" //nolint:gosec

	ubiSharedCAPath    = "/etc/pki/ca-trust/source/anchors/"
	ubiUpdateCmd       = "/usr/bin/update-ca-trust"
	debianSharedCAPath = "/usr/local/share/ca-certificates/"
	debianUpdateCmd    = "/usr/sbin/update-ca-certificates"
)

var (
	// TODO: Decrease back to 350Mi once https://github.com/elastic/elastic-agent/issues/4730 is addressed
	defaultResources = corev1.ResourceRequirements{
		Limits: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceMemory: resource.MustParse("400Mi"),
			corev1.ResourceCPU:    resource.MustParse("200m"),
		},
		Requests: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceMemory: resource.MustParse("400Mi"),
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
		FleetEnrollmentToken:             {},
		FleetServerElasticsearchUsername: {},
		FleetServerElasticsearchPassword: {},
	}
)

func buildPodTemplate(params Params, fleetCerts *certificates.CertificatesSecret, fleetToken EnrollmentAPIKey, configHash hash.Hash32) (corev1.PodTemplateSpec, error) {
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
		if builder, err = amendBuilderForFleetMode(params, fleetCerts, fleetToken, builder, configHash); err != nil {
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
	}

	v, err := version.Parse(spec.Version)
	if err != nil {
		return corev1.PodTemplateSpec{}, err // error unlikely and should have been caught during validation
	}
	// volume with agent data path if version > 7.15 (available since 7.13 but non-functional as agent tries to fork child
	// processes in data path directory and hostPath volumes are always mounted non-exec)
	if v.GTE(version.MinFor(7, 15, 0)) {
		vols = append(vols, createDataVolume(params))
	}

	// all volumes with CAs of direct associations
	caAssocVols, err := getVolumesFromAssociations(params.Agent.GetAssociations())
	if err != nil {
		return corev1.PodTemplateSpec{}, err
	}
	vols = append(vols, caAssocVols...)

	agentLabels := maps.Merge(params.Agent.GetIdentityLabels(), map[string]string{
		VersionLabelName: spec.Version})

	annotations := map[string]string{
		ConfigHashAnnotationName: fmt.Sprint(configHash.Sum32()),
	}

	builder = builder.
		WithLabels(agentLabels).
		WithAnnotations(annotations).
		WithDockerImage(spec.Image, container.ImageRepository(container.AgentImage, v)).
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

func amendBuilderForFleetMode(params Params, fleetCerts *certificates.CertificatesSecret, fleetToken EnrollmentAPIKey, builder *defaults.PodTemplateBuilder, configHash hash.Hash) (*defaults.PodTemplateBuilder, error) {
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

	// ES, Kibana and FleetServer connection info are injected using environment variables
	builder, err = applyEnvVars(params, fleetToken, fleetCerts, builder)
	if err != nil {
		return nil, err
	}

	if params.Agent.Spec.FleetServerEnabled {
		builder = builder.WithPorts([]corev1.ContainerPort{{Name: params.Agent.Spec.HTTP.Protocol(), ContainerPort: FleetServerPort, Protocol: corev1.ProtocolTCP}})

		// Only add certificate volumes if TLS is enabled.
		if params.Agent.Spec.HTTP.TLS.Enabled() {
			// ECK creates CA and a certificate for Fleet Server to use. This volume contains those.
			builder = builder.WithVolumeLikes(
				volume.NewSecretVolumeWithMountPath(
					fleetCerts.Name,
					FleetCertsVolumeName,
					FleetCertsMountPath,
				))
		}
	}

	builder = builder.
		WithResources(defaultFleetResources).
		// needed to pick up fleet-setup.yml correctly
		WithEnv(corev1.EnvVar{Name: "CONFIG_PATH", Value: "/usr/share/elastic-agent"})

	return builder, nil
}

func applyEnvVars(params Params, fleetToken EnrollmentAPIKey, certs *certificates.CertificatesSecret, builder *defaults.PodTemplateBuilder) (*defaults.PodTemplateBuilder, error) {
	fleetModeEnvVars, err := getFleetModeEnvVars(params.Context, params.Agent, params.Client, fleetToken, certs)
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
			Labels:    labels.AddCredentialsLabel(params.Agent.GetIdentityLabels()),
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
	} else if _, err := reconciler.ReconcileSecret(params.Context, params.Client, envVarsSecret, &params.Agent); err != nil {
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
	} else if params.Agent.Spec.FleetServerRef.IsDefined() {
		// As the reference chain is: Elastic Agent ---> Fleet Server ---> Elasticsearch,
		// we need first to identify the Fleet Server and then identify its reference to Elasticsearch.
		fsAssociation, err := association.SingleAssociationOfType(params.Agent.GetAssociations(), commonv1.FleetServerAssociationType)
		if err != nil {
			return nil, err
		}

		fsRef := fsAssociation.AssociationRef()
		if fsRef.IsExternal() {
			// the Fleet Server is not managed by ECK, no transitive ES association to get to apply
			return nil, nil
		}

		fs := agentv1alpha1.Agent{}
		if err := params.Client.Get(params.Context, fsRef.NamespacedName(), &fs); err != nil {
			return nil, pkgerrors.Wrap(err, "while fetching associated fleet server")
		}

		esAssociation, err = association.SingleAssociationOfType(fs.GetAssociations(), commonv1.ElasticsearchAssociationType)
		if err != nil {
			return nil, err
		}
	}
	return esAssociation, nil
}

func applyRelatedEsAssoc(agent agentv1alpha1.Agent, esAssociation commonv1.Association, builder *defaults.PodTemplateBuilder) (*defaults.PodTemplateBuilder, error) {
	if esAssociation == nil {
		return builder, nil
	}

	// no ES CA to configure, skip
	assocConf, err := esAssociation.AssociationConf()
	if err != nil {
		return nil, err
	}
	if !assocConf.CAIsConfigured() {
		return builder, nil
	}
	builder = builder.WithVolumeLikes(volume.NewSecretVolumeWithMountPath(
		assocConf.GetCASecretName(),
		fmt.Sprintf("%s-certs", esAssociation.AssociationType()),
		certificatesDir(esAssociation),
	))

	// If agent is set to run as root, then we will add the Elasticsearch CA to the
	// pod's trusted CA store as both FLEET_CA and ELASTICSEARCH_CA environment variables
	// are not respected by Agent in fleet mode. If we're not running as root, then we'll document the procedure
	// to add the Elasticsearch CA to the Kibana's xpack output configuration pertaining to Fleet.
	//
	// For historic purposes (https://github.com/elastic/beats/pull/26529). Agent didn't respect
	// FLEET_CA until 7.14.0, which is the lowest valid version we support of Agent + Fleet.
	if runningAsRoot(agent) {
		cmd := trustCAScript(path.Join(certificatesDir(esAssociation), CAFileName))
		return builder.WithCommand([]string{"/usr/bin/env", "bash", "-c", cmd}), nil
	}
	return builder, nil
}

func runningAsRoot(agent agentv1alpha1.Agent) bool {
	switch {
	case agent.Spec.DaemonSet != nil:
		return runningContainerAsRoot(agent.Spec.DaemonSet.PodTemplate)
	case agent.Spec.Deployment != nil:
		return runningContainerAsRoot(agent.Spec.Deployment.PodTemplate)
	case agent.Spec.StatefulSet != nil:
		return runningContainerAsRoot(agent.Spec.StatefulSet.PodTemplate)
	default:
		return false
	}
}

func runningContainerAsRoot(podTemplate corev1.PodTemplateSpec) bool {
	if podTemplate.Spec.SecurityContext != nil &&
		podTemplate.Spec.SecurityContext.RunAsUser != nil &&
		*podTemplate.Spec.SecurityContext.RunAsUser == 0 {
		return true
	}
	for _, podContainer := range podTemplate.Spec.Containers {
		if podContainer.SecurityContext != nil && podContainer.SecurityContext.RunAsUser != nil {
			if *podContainer.SecurityContext.RunAsUser == 0 {
				return true
			}
		}
	}
	return false
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
		// the Kibana association is only used by the operator to interact with the Kibana Fleet API but
		// not by the individual Elastic Agent Pods. There is therefore no need to mount the Kibana certificate secret.
		if assoc.AssociationType() == commonv1.KibanaAssociationType {
			continue
		}
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
		ref.NameOrSecretName(),
	)
}

func getFleetModeEnvVars(
	ctx context.Context,
	agent agentv1alpha1.Agent,
	client k8s.Client,
	fleetToken EnrollmentAPIKey,
	certs *certificates.CertificatesSecret,
) (map[string]string, error) {
	result := map[string]string{}

	for _, f := range []func(agentv1alpha1.Agent) (map[string]string, error){
		getFleetSetupKibanaEnvVars(fleetToken),
		getFleetSetupFleetEnvVars(client, fleetToken, certs),
		getFleetSetupFleetServerEnvVars(ctx, client),
	} {
		envVars, err := f(agent)
		if err != nil {
			return nil, err
		}
		result = maps.Merge(result, envVars)
	}

	return result, nil
}

func getFleetSetupKibanaEnvVars(fleetToken EnrollmentAPIKey) func(agent agentv1alpha1.Agent) (map[string]string, error) {
	return func(agent agentv1alpha1.Agent) (map[string]string, error) {
		if !agent.Spec.KibanaRef.IsDefined() {
			return map[string]string{}, nil
		}

		if fleetToken.isEmpty() {
			return nil, errors.New("fleet enrollment token must not be empty, potential programmer error")
		}

		envVars := map[string]string{
			FleetEnrollmentToken: fleetToken.APIKey,
		}

		return envVars, nil
	}
}

func getFleetSetupFleetEnvVars(client k8s.Client, fleetToken EnrollmentAPIKey, fleetCerts *certificates.CertificatesSecret) func(agent agentv1alpha1.Agent) (map[string]string, error) {
	return func(agent agentv1alpha1.Agent) (map[string]string, error) {
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
			if agent.Spec.HTTP.TLS.Enabled() && fleetCerts.HasCA() {
				fleetCfg[FleetCA] = path.Join(FleetCertsMountPath, certificates.CAFileName)
			}
			// Fleet Server needs a policy ID to bootstrap itself unless a policy marked as default is used.
			if agent.Spec.KibanaRef.IsDefined() && !fleetToken.isEmpty() {
				fleetCfg[FleetServerPolicyID] = fleetToken.PolicyID
			}
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
			fleetURL := assocConf.GetURL()
			fleetCfg[FleetURL] = fleetURL

			if !strings.HasPrefix(fleetURL, "https://") {
				fleetCfg[FleetInsecure] = "true"
			}

			if assocConf.GetCACertProvided() {
				fleetCfg[FleetCA] = path.Join(certificatesDir(assoc), CAFileName)
			}
		}

		return fleetCfg, nil
	}
}

func getFleetSetupFleetServerEnvVars(ctx context.Context, client k8s.Client) func(agent agentv1alpha1.Agent) (map[string]string, error) {
	return func(agent agentv1alpha1.Agent) (map[string]string, error) {
		if !agent.Spec.FleetServerEnabled {
			return map[string]string{}, nil
		}

		fleetServerCfg := map[string]string{
			FleetServerEnable: "true",
		}

		if agent.Spec.HTTP.TLS.Enabled() {
			fleetServerCfg[FleetServerCert] = path.Join(FleetCertsMountPath, certificates.CertFileName)
			fleetServerCfg[FleetServerCertKey] = path.Join(FleetCertsMountPath, certificates.KeyFileName)
		} else {
			fleetServerCfg[FleetServerInsecureHTTP] = "true"
			fleetServerCfg[FleetServerHost] = "0.0.0.0"
			fleetServerCfg[FleetServerPortEnv] = fmt.Sprintf("%d", FleetServerPort)
		}

		esExpected := len(agent.Spec.ElasticsearchRefs) > 0 && agent.Spec.ElasticsearchRefs[0].IsDefined()
		if esExpected {
			esConnectionSettings, _, err := extractPodConnectionSettings(ctx, agent, client, commonv1.ElasticsearchAssociationType)
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
			if esConnectionSettings.caFileName != "" {
				fleetServerCfg[FleetServerElasticsearchCA] = esConnectionSettings.caFileName
			}
		}

		return fleetServerCfg, nil
	}
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
