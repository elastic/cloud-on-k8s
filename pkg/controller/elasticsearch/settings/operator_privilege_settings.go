// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package settings

import (
	"fmt"
	"strings"
)

type OperatorRealmType string

const (
	OperatorRealmTypeFile           OperatorRealmType = "file"
	OperatorRealmTypeServiceAccount OperatorRealmType = "_service_account"
	OperatorRealmTypeJWT            OperatorRealmType = "jwt"

	OperatorUsersSettingsFileName = "operator_users.yml"
	TokenAuthType                 = "token"
	RealmAuthType                 = "realm"
)

type OperatorPrivilegesSetting struct {
	Usernames []string          `yaml:"usernames"`
	RealmType OperatorRealmType `yaml:"realm_type"`
	RealmName string            `yaml:"realm_name,omitempty"`
	AuthType  string            `yaml:"auth_type,omitempty"`
	// Token auth type specific fields
	TokenSource string   `yaml:"token_source,omitempty"`
	TokenNames  []string `yaml:"token_names,omitempty"`
}

type OperatorPrivilegesSettings struct {
	Operator []OperatorPrivilegesSetting `yaml:"operator"`
}

type OperatorAccount struct {
	Names     []string          `mapstructure:"names" validate:"required"`
	RealmType OperatorRealmType `mapstructure:"realm_type" validate:"required"`
}

func NewOperatorPrivilegesSettings(operatorUsernames []OperatorAccount, serviceAccountTokenNames []string) (OperatorPrivilegesSettings, error) {
	var operatorPrivilegesSettings OperatorPrivilegesSettings
	// Build a map from username to token names based on the provided tokenNames list
	namespacedServicesToTokenMap := make(map[string][]string)
	for _, serviceAccountTokenName := range serviceAccountTokenNames {
		key, name := splitNamespacedServiceTokenName(serviceAccountTokenName)
		if key != "" && name != "" {
			namespacedServicesToTokenMap[key] = append(namespacedServicesToTokenMap[key], name)
		}
	}

	for _, operatorUsername := range operatorUsernames {
		switch operatorUsername.RealmType {
		case OperatorRealmTypeServiceAccount:
			// verify the corresponding service account token was added to ElasticsearchAppConfig CR
			for _, username := range operatorUsername.Names {
				if tokenNames, exists := namespacedServicesToTokenMap[username]; exists {
					operatorPrivilegesSettings.Operator = append(operatorPrivilegesSettings.Operator, OperatorPrivilegesSetting{
						Usernames:   []string{username},
						RealmType:   operatorUsername.RealmType,
						AuthType:    TokenAuthType,
						TokenSource: "file",
						TokenNames:  tokenNames,
					})
				}
			}
		case OperatorRealmTypeFile:
			operatorPrivilegesSettings.Operator = append(operatorPrivilegesSettings.Operator, OperatorPrivilegesSetting{
				Usernames: operatorUsername.Names,
				RealmType: operatorUsername.RealmType,
				AuthType:  RealmAuthType,
			})
		case OperatorRealmTypeJWT:
			operatorPrivilegesSettings.Operator = append(operatorPrivilegesSettings.Operator, OperatorPrivilegesSetting{
				Usernames: operatorUsername.Names,
				RealmType: operatorUsername.RealmType,
				RealmName: "jwt1",
			})
		default:
			return OperatorPrivilegesSettings{}, fmt.Errorf("unknown realm type %s, known realm types are %s and %s",
				operatorUsername.RealmType, OperatorRealmTypeServiceAccount, OperatorRealmTypeFile)
		}
	}

	return operatorPrivilegesSettings, nil
}

// splitNamespacedServiceTokenName parses a service account token name of the form
// "<namespace>/<service>/<token>" and returns the map key ("<namespace>/<service>") and token name.
func splitNamespacedServiceTokenName(tokenName string) (key string, token string) {
	parts := strings.Split(tokenName, "/")
	key = fmt.Sprintf("%s/%s", parts[0], parts[1])
	return key, parts[2]
}
