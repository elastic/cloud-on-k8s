// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package flags

import (
	"time"
)

const (
	// root flags
	TagFlag               = "tag"
	DryRunFlag            = "dry-run"
	EnableVaultFlag       = "enable-vault"
	VaultAddressFlag      = "vault-addr"
	VaultTokenFlag        = "vault-token"
	RedhatVaultSecretFlag = "redhat-vault-secret"
	GithubVaultSecretFlag = "github-vault-secret"

	// bundle command flags
	DirFlag                        = "dir"
	SupportedOpenshiftVersionsFlag = "supported-openshift-versions"
	SubmitPullRequestFlag          = "submit-pull-request"
	GithubTokenFlag                = "github-token"
	GithubUsernameFlag             = "github-username"
	GithubFullnameFlag             = "github-fullname"
	GithubEmailFlag                = "github-email"
	DeleteTempDirectoryFlag        = "delete-temp-directory"
	ErrRequiredIfEnabled           = "%s is required if %s is enabled"

	// container command flags
	ApiKeyFlags          = "api-key"
	RegistryPasswordFlag = "registry-password"
	ProjectIDFlag        = "project-id"
	ForceFlag            = "force"
	ScanTimeoutFlag      = "scan-timeout"

	// operatorhub command flags
	PrevVersionFlag  = "prev-version"
	StackVersionFlag = "stack-version"
	ConfFlag         = "conf"
	YamlManifestFlag = "yaml-manifest"
	TemplatesFlag    = "templates"
	RootPathFlag     = "root-path"
)

var (
	// bundle command variables
	Dir                        string
	SupportedOpenshiftVersions string
	SubmitPullRequest          bool
	GithubToken                string
	GithubUsername             string
	GithubFullname             string
	GithubEmail                string
	DeleteTempDirectory        bool

	// container command variables
	ApiKey           string
	RegistryPassword string
	ProjectID        string
	Force            bool
	ScanTimeout      time.Duration

	// root command variables
	Tag               string
	DryRun            bool
	EnableVault       bool
	VaultAddress      string
	VaultToken        string
	RedhatVaultSecret string
	GithubVaultSecret string

	// operatorhub command variables
	PreviousVersion string
	StackVersion    string
	Conf            string
	YamlManifest    []string
	Templates       string
	RootPath        string
)
