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

	if params.Agent.Spec.Mode == agentv1alpha1.AgentFleetMode {
		fleetSetupCfgBytes, err := buildFleetSetupConfig(params)
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

	if userConfig != nil {
		if err = cfg.MergeWith(userConfig); err != nil {
			return nil, err
		}
	}

	return cfg.Render()
}

func buildOutputConfig(params Params) (*settings.CanonicalConfig, error) {
	if params.Agent.Spec.Mode == agentv1alpha1.AgentFleetMode {
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

func buildFleetSetupConfig(params Params) ([]byte, error) {
	kibanaRefDefined, kbHost, kbCA, kbUsername, kbPassword, err := extractConnectionSettings(params.Agent.Spec.KibanaRef.IsDefined(), params.Agent, params.Client, commonv1.KibanaAssociationType)
	if err != nil {
		return nil, err
	}

	if params.Agent.Spec.EnableFleetServer {
		esExpected := len(params.Agent.Spec.ElasticsearchRefs) > 0 && params.Agent.Spec.ElasticsearchRefs[0].IsDefined()
		_, esHost, esCA, esUsername, esPassword, err := extractConnectionSettings(esExpected, params.Agent, params.Client, commonv1.ElasticsearchAssociationType)
		if err != nil {
			return nil, err
		}

		cfg, err := settings.NewCanonicalConfigFrom(map[string]interface{}{
			"fleet": map[string]interface{}{
				"ca":     path.Join(FleetCertMountPath, certificates.CAFileName),
				"enroll": kibanaRefDefined,
				"url":    fmt.Sprintf("https://%s.%s.svc:8220", HttpServiceName(params.Agent.Name), params.Agent.Namespace),
			},
			"fleet_server": map[string]interface{}{
				"enable":   true,
				"cert":     path.Join(FleetCertMountPath, certificates.CertFileName),
				"cert_key": path.Join(FleetCertMountPath, certificates.KeyFileName),
				"elasticsearch": map[string]interface{}{
					"ca":       esCA,
					"host":     esHost,
					"username": esUsername,
					"password": esPassword,
				},
			},
			"kibana": map[string]interface{}{
				"fleet": map[string]interface{}{
					"ca":       kbCA,
					"host":     kbHost,
					"password": kbPassword,
					"setup":    kibanaRefDefined,
					"username": kbUsername,
				},
			},
		})
		if err != nil {
			return nil, err
		}

		return cfg.Render()
	} else {
		var fsHost, fsCA string
		if params.Agent.Spec.FleetServerRef.IsDefined() {
			assoc := getAssociationOfType(params.Agent.GetAssociations(), commonv1.FleetServerAssociationType)
			if assoc != nil {
				fsCA = path.Join(certificatesDir(assoc), CAFileName)
				fsHost = assoc.AssociationConf().GetURL()
			}
		}

		cfg, err := settings.NewCanonicalConfigFrom(map[string]interface{}{
			"fleet": map[string]interface{}{
				"ca":     fsCA,
				"enroll": kibanaRefDefined,
				"url":    fsHost,
			},
			"fleet_server": map[string]interface{}{
				"enable": false,
			},
			"kibana": map[string]interface{}{
				"fleet": map[string]interface{}{
					"ca":       kbCA,
					"host":     kbHost,
					"password": kbPassword,
					"setup":    false,
					"username": kbUsername, // todo check if those can be overridden by env vars if they are just empty strings
				},
			},
		})
		if err != nil {
			return nil, err
		}

		return cfg.Render()
	}
}

func extractConnectionSettings(
	isExpected bool,
	agent agentv1alpha1.Agent,
	client k8s.Client,
	associationType commonv1.AssociationType,
) (expected bool, host, ca, username, password string, err error) {
	if !isExpected {
		return isExpected, "", "", "", "", nil
	}

	assoc := getAssociationOfType(agent.GetAssociations(), associationType)
	if assoc == nil {
		return true, "",
			"",
			"",
			"",
			fmt.Errorf("association %s not found in %d associations", associationType, len(agent.GetAssociations()))
	}

	username, password, err = association.ElasticsearchAuthSettings(client, assoc)
	if err != nil {
		return true, "", "", "", "", err
	}

	caPath := path.Join(certificatesDir(assoc), CAFileName)
	return true, assoc.AssociationConf().GetURL(), caPath, username, password, nil
}

func getAssociationOfType(
	associations []commonv1.Association,
	associationType commonv1.AssociationType,
) commonv1.Association {
	for _, assoc := range associations {
		if assoc.AssociationType() != associationType {
			continue
		}
		return assoc
	}
	return nil
}
