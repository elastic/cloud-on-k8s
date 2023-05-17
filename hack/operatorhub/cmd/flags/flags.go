// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package flags

import (
	"time"
)

const (
	// common missing flag error
	RequiredErrFmt = "%s is required"

	// root flags
	ConfFlag              = "conf"
	DryRunFlag            = "dry-run"
	EnableVaultFlag       = "enable-vault"
	VaultAddressFlag      = "vault-addr"
	VaultTokenFlag        = "vault-token"
	RedhatVaultSecretFlag = "redhat-vault-secret"
	GithubVaultSecretFlag = "github-vault-secret" //nolint:gosec

	// bundle command flags
	DirFlag                 = "dir"
	GithubTokenFlag         = "github-token"
	GithubUsernameFlag      = "github-username"
	GithubFullnameFlag      = "github-fullname"
	GithubEmailFlag         = "github-email"
	DeleteTempDirectoryFlag = "delete-temp-directory"

	// container command flags
	APIKeyFlags          = "api-key"
	RegistryPasswordFlag = "registry-password"
	ProjectIDFlag        = "project-id"
	ForceFlag            = "force"
	ScanTimeoutFlag      = "scan-timeout"

	// operatorhub command flags
	YamlManifestFlag = "yaml-manifest"
	TemplatesFlag    = "templates"
	RootPathFlag     = "root-path"
)

var (
	// bundle command variables
	Dir                 string
	GithubToken         string
	GithubUsername      string
	GithubFullname      string
	GithubEmail         string
	DeleteTempDirectory bool

	// container command variables
	APIKey           string
	RegistryPassword string
	ProjectID        string
	Force            bool
	ScanTimeout      time.Duration

	// root command variables
	ConfigPath        string
	DryRun            bool
	EnableVault       bool
	GithubVaultSecret string
	RedhatVaultSecret string
	VaultAddress      string
	VaultToken        string

	// operatorhub command variables
	Conf         *Config
	YamlManifest []string
	Templates    string
	RootPath     string
)

// Config is the configuration that matches the config.yaml
type Config struct {
	NewVersion                   string `json:"newVersion"`
	PrevVersion                  string `json:"prevVersion"`
	StackVersion                 string `json:"stackVersion"`
	MinSupportedOpenshiftVersion string `json:"minSupportedOpenShiftVersion"`
	CRDs                         []struct {
		Name        string `json:"name"`
		DisplayName string `json:"displayName"`
		Description string `json:"description"`
	} `json:"crds"`
	Packages []struct {
		OutputPath          string `json:"outputPath"`
		PackageName         string `json:"packageName"`
		DistributionChannel string `json:"distributionChannel"`
		OperatorRepo        string `json:"operatorRepo"`
		UbiOnly             bool   `json:"ubiOnly"`
		DigestPinning       bool   `json:"digestPinning"`
	} `json:"packages"`
}

// HasDigestPinning will return true if any package
// within the configuration has DigestPinning enabled.
func (c *Config) HasDigestPinning() bool {
	if c == nil {
		return false
	}
	for _, pkg := range c.Packages {
		if pkg.DigestPinning {
			return true
		}
	}
	return false
}
