// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package agent

import (
	"errors"
	"fmt"
	"hash"
	"path"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	agentv1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/agent/v1alpha1"
	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

// Below are keys used in fleet-setup.yaml file.
const (
	// FleetSetupKibanaKey is a key used in fleet-setup.yaml file to denote kibana part of the configuration.
	FleetSetupKibanaKey = "kibana"
	// FleetSetupFleetServerKey is a key used in fleet-setup.yaml file to denote fleet server part of the configuration.
	FleetSetupFleetServerKey = "fleet_server"
	// FleetSetupFleetKey is a key used in fleet-setup.yaml file to denote fleet part of the configuration.
	FleetSetupFleetKey = "fleet"
)

type connectionSettings struct {
	host, ca, username, password string
}

func reconcileConfig(params Params, configHash hash.Hash) *reconciler.Results {
	defer tracing.Span(&params.Context)()
	results := reconciler.NewResult(params.Context)

	cfgBytes, err := buildConfig(params)
	if err != nil {
		return results.WithError(err)
	}

	cfgData := map[string][]byte{
		ConfigFileName: cfgBytes,
	}

	if params.Agent.Spec.FleetModeEnabled() {
		fleetSetupCfgBytes, err := buildFleetSetupConfig(params.Agent, params.Client)
		if err != nil {
			return results.WithError(err)
		}

		cfgData[FleetSetupFileName] = fleetSetupCfgBytes
		_, _ = configHash.Write(fleetSetupCfgBytes)
	}

	expected := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: params.Agent.Namespace,
			Name:      ConfigSecretName(params.Agent.Name),
			Labels:    common.AddCredentialsLabel(NewLabels(params.Agent)),
		},
		Data: cfgData,
	}

	if _, err = reconciler.ReconcileSecret(params.Client, expected, &params.Agent); err != nil {
		return results.WithError(err)
	}

	_, _ = configHash.Write(cfgBytes)

	return results
}

func buildConfig(params Params) ([]byte, error) {
	cfg, err := buildOutputConfig(params)
	if err != nil {
		return nil, err
	}

	// get user config from `config` or `configRef`
	userConfig, err := getUserConfig(params)
	if err != nil {
		return nil, err
	}

	if err = cfg.MergeWith(userConfig); err != nil {
		return nil, err
	}

	return cfg.Render()
}

func buildOutputConfig(params Params) (*settings.CanonicalConfig, error) {
	if params.Agent.Spec.FleetModeEnabled() {
		// in fleet mode outputs are owned by fleet
		return settings.NewCanonicalConfig(), nil
	}

	allAssociations := params.Agent.GetAssociations()

	var esAssociations []commonv1.Association
	for _, assoc := range allAssociations {
		if assoc.AssociationType() == commonv1.ElasticsearchAssociationType {
			esAssociations = append(esAssociations, assoc)
		}
	}

	for _, assoc := range esAssociations {
		if !assoc.AssociationConf().IsConfigured() {
			return settings.NewCanonicalConfig(), nil
		}
	}

	outputs := map[string]interface{}{}
	for i, assoc := range esAssociations {
		username, password, err := association.ElasticsearchAuthSettings(params.Client, assoc)
		if err != nil {
			return settings.NewCanonicalConfig(), err
		}

		output := map[string]interface{}{
			"type":     "elasticsearch",
			"username": username,
			"password": password,
			"hosts":    []string{assoc.AssociationConf().GetURL()},
		}
		if assoc.AssociationConf().GetCACertProvided() {
			output["ssl.certificate_authorities"] = []string{path.Join(certificatesDir(assoc), CAFileName)}
		}

		outputName := params.Agent.Spec.ElasticsearchRefs[i].OutputName
		if outputName == "" {
			if len(esAssociations) > 1 {
				return settings.NewCanonicalConfig(), errors.New("output is not named and there is more than one specified")
			}
			outputName = "default"
		}
		outputs[outputName] = output
	}

	return settings.NewCanonicalConfigFrom(map[string]interface{}{
		"outputs": outputs,
	})
}

// getUserConfig extracts the config either from the spec `config` field or from the Secret referenced by spec
// `configRef` field.
func getUserConfig(params Params) (*settings.CanonicalConfig, error) {
	if params.Agent.Spec.Config != nil {
		return settings.NewCanonicalConfigFrom(params.Agent.Spec.Config.Data)
	}
	return common.ParseConfigRef(params, &params.Agent, params.Agent.Spec.ConfigRef, ConfigFileName)
}

func buildFleetSetupConfig(agent agentv1alpha1.Agent, client k8s.Client) ([]byte, error) {
	cfgMap := map[string]interface{}{}

	for _, cfgPart := range []struct {
		key string
		f   func(agentv1alpha1.Agent, k8s.Client) (map[string]interface{}, error)
	}{
		{key: FleetSetupKibanaKey, f: buildFleetSetupKibanaConfig},
		{key: FleetSetupFleetKey, f: buildFleetSetupFleetConfig},
		{key: FleetSetupFleetServerKey, f: buildFleetSetupFleetServerConfig},
	} {
		cfg, err := cfgPart.f(agent, client)
		if err != nil {
			return nil, err
		}
		if cfg != nil {
			cfgMap[cfgPart.key] = cfg
		}
	}

	cfg, err := settings.NewCanonicalConfigFrom(cfgMap)
	if err != nil {
		return nil, err
	}

	return cfg.Render()
}

func buildFleetSetupKibanaConfig(agent agentv1alpha1.Agent, client k8s.Client) (map[string]interface{}, error) {
	if agent.Spec.KibanaRef.IsDefined() {
		kbConnectionSettings, err := extractConnectionSettings(agent, client, commonv1.KibanaAssociationType)
		if err != nil {
			return nil, err
		}

		fleet := map[string]interface{}{
			"host":     kbConnectionSettings.host,
			"password": kbConnectionSettings.password,
			"setup":    agent.Spec.KibanaRef.IsDefined(),
			"username": kbConnectionSettings.username,
		}

		// don't set ca key if ca is not available
		if kbConnectionSettings.ca != "" {
			fleet["ca"] = kbConnectionSettings.ca
		}

		return map[string]interface{}{
			"fleet": fleet,
		}, nil
	}

	return nil, nil
}

func buildFleetSetupFleetConfig(agent agentv1alpha1.Agent, client k8s.Client) (map[string]interface{}, error) {
	fleetCfg := map[string]interface{}{}

	if agent.Spec.KibanaRef.IsDefined() {
		fleetCfg["enroll"] = true
	}

	if agent.Spec.FleetServerEnabled {
		fleetURL, err := association.ServiceURL(
			client,
			types.NamespacedName{Namespace: agent.Namespace, Name: HTTPServiceName(agent.Name)},
			agent.Spec.HTTP.Protocol(),
		)
		if err != nil {
			return nil, err
		}

		fleetCfg["ca"] = path.Join(FleetCertsMountPath, certificates.CAFileName)
		fleetCfg["url"] = fleetURL
	} else if agent.Spec.FleetServerRef.IsDefined() {
		assoc, err := association.SingleAssociationOfType(agent.GetAssociations(), commonv1.FleetServerAssociationType)
		if err != nil {
			return nil, err
		}

		if assoc != nil {
			fleetCfg["ca"] = path.Join(certificatesDir(assoc), CAFileName)
			fleetCfg["url"] = assoc.AssociationConf().GetURL()
		}
	}
	return fleetCfg, nil
}

func buildFleetSetupFleetServerConfig(agent agentv1alpha1.Agent, client k8s.Client) (map[string]interface{}, error) {
	if !agent.Spec.FleetServerEnabled {
		return map[string]interface{}{}, nil
	}

	fleetServerCfg := map[string]interface{}{
		"enable":   true,
		"cert":     path.Join(FleetCertsMountPath, certificates.CertFileName),
		"cert_key": path.Join(FleetCertsMountPath, certificates.KeyFileName),
	}

	esExpected := len(agent.Spec.ElasticsearchRefs) > 0 && agent.Spec.ElasticsearchRefs[0].IsDefined()
	if esExpected {
		esConnectionSettings, err := extractConnectionSettings(agent, client, commonv1.ElasticsearchAssociationType)
		if err != nil {
			return nil, err
		}

		elasticsearch := map[string]interface{}{
			"host":     esConnectionSettings.host,
			"username": esConnectionSettings.username,
			"password": esConnectionSettings.password,
		}

		// don't set ca key if ca is not available
		if esConnectionSettings.ca != "" {
			elasticsearch["ca"] = esConnectionSettings.ca
		}

		fleetServerCfg["elasticsearch"] = elasticsearch
	}

	return fleetServerCfg, nil
}

func extractConnectionSettings(
	agent agentv1alpha1.Agent,
	client k8s.Client,
	associationType commonv1.AssociationType,
) (connectionSettings, error) {
	assoc, err := association.SingleAssociationOfType(agent.GetAssociations(), associationType)
	if err != nil {
		return connectionSettings{}, err
	}

	if assoc == nil {
		errTemplate := "association of type %s not found in %d associations"
		return connectionSettings{}, fmt.Errorf(errTemplate, associationType, len(agent.GetAssociations()))
	}

	username, password, err := association.ElasticsearchAuthSettings(client, assoc)
	if err != nil {
		return connectionSettings{}, err
	}

	ca := ""
	if assoc.AssociationConf().GetCACertProvided() {
		ca = path.Join(certificatesDir(assoc), CAFileName)
	}

	return connectionSettings{
		host:     assoc.AssociationConf().GetURL(),
		ca:       ca,
		username: username,
		password: password,
	}, err
}
