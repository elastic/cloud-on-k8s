// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package runner

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"text/template"
	"time"

	"github.com/elastic/cloud-on-k8s/v3/hack/deployer/exec"
	"github.com/elastic/cloud-on-k8s/v3/hack/deployer/runner/bucket"
	"github.com/elastic/cloud-on-k8s/v3/hack/deployer/runner/env"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/set"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/vault"
)

const (
	OCPDriverID                     = "ocp"
	OCPVaultPath                    = "ci-ocp-k8s-operator"
	OCPServiceAccountVaultFieldName = "service-account"
	OCPPullSecretFieldName          = "pull-secret"
	OCPStateBucket                  = "eck-deployer-ocp-clusters-state"
	DefaultOCPRunConfigTemplate     = `id: ocp-dev
overrides:
  clusterName: %s-dev-cluster
  ocp:
    gCloudProject: %s
`

	OcpInstallerConfigTemplate = `apiVersion: v1
baseDomain: {{.BaseDomain}}
compute:
- hyperthreading: Enabled
  name: worker
  platform:
    gcp:
      type: {{.MachineType}}
  replicas: {{.NodeCount}}
controlPlane:
  hyperthreading: Enabled
  name: master
  platform:
    gcp:
      type: {{.MachineType}}
  replicas: {{.NodeCount}}
metadata:
  creationTimestamp: null
  name: {{.ClusterName}}
networking:
  clusterNetwork:
  - cidr: 10.128.0.0/14
    hostPrefix: 23
  machineCIDR: 10.0.0.0/16
  networkType: OVNKubernetes
  serviceNetwork:
  - 172.30.0.0/16
platform:
  gcp:
    projectID: {{.GCloudProject}}
    region: {{.Region}}
    userLabels:{{range $key, $value := .UserLabels}}
    - key: {{$key}}
      value: {{$value}}{{end}}
pullSecret: '{{.PullSecret}}'`
)

// ocpServiceAccountRE extracts the infra ID from an OCP service account email.
// OCP creates service accounts per cluster with a single-char role suffix:
//
//	{infraID}-m@{project}.iam.gserviceaccount.com  (master)
//	{infraID}-w@{project}.iam.gserviceaccount.com  (worker)
//	{infraID}-b@{project}.iam.gserviceaccount.com  (bootstrap)
//
// When the infra ID is too long (GCP enforces a 30-char limit on SA IDs),
// the role suffix is omitted and the SA name is the infra ID itself:
//
//	{infraID}@{project}.iam.gserviceaccount.com
var ocpServiceAccountRE = regexp.MustCompile(`^(.+?)(?:-[bmw])?@`)

func init() {
	drivers[OCPDriverID] = &OCPDriverFactory{}
}

type OCPDriverFactory struct{}

type runtimeState struct {
	// Authenticated tracks authentication against the GCloud API to avoid double authentication.
	Authenticated bool
	// SafeToDeleteWorkdir indicates that the installer state has been uploaded successfully to the storage bucket or is
	// otherwise not needed anymore.
	SafeToDeleteWorkdir bool
	// ClusterStateDir is the effective work dir containing the OCP installer state. Derived from plan.OCP.Workdir.
	ClusterStateDir string
	// ClientImage is the name of the installer client image.
	ClientImage string
}

type OCPDriver struct {
	plan         Plan
	runtimeState runtimeState
	vaultClient  vault.Client
}

func (*OCPDriverFactory) Create(plan Plan) (Driver, error) {
	c, err := vault.NewClient()
	if err != nil {
		return nil, err
	}
	return &OCPDriver{
		plan:        plan,
		vaultClient: c,
	}, nil
}

func (d *OCPDriver) setup() []func() error {
	return []func() error{
		d.ensureWorkDir,
		d.authToGCP,
		d.ensurePullSecret,
		d.downloadClusterState,
	}
}

func (d *OCPDriver) Execute() error {
	// client image requires a plan which we don't have in GetCredentials
	setup := append(d.setup(), d.ensureClientImage)

	if err := run(setup); err != nil {
		return err
	}

	defer func() {
		_ = d.removeWorkDir()
	}()

	clusterStatus := d.currentStatus()

	switch d.plan.Operation {
	case DeleteAction:
		// Track bucket deletion errors separately: cluster deletion should proceed even if bucket
		// deletion fails, but the error must still be returned so the exit code is non-zero.
		bucketErr := deleteBucketIfConfigured(d.plan, d.newBucketManager)
		if bucketErr != nil {
			log.Printf("warning: bucket deletion failed, will continue with cluster deletion: %v", bucketErr)
		}
		if clusterStatus != NotFound {
			// always attempt a deletion
			if err := d.delete(); err != nil {
				return errors.Join(err, bucketErr)
			}
		} else {
			log.Printf("Not deleting as cluster doesn't exist")
		}
		if bucketErr != nil {
			return bucketErr
		}
	case CreateAction:
		if clusterStatus == Running {
			log.Printf("Not creating as cluster exists")
			// rsync sometimes get stuck this makes sure we retry upload on repeated create invocations
			if err := d.uploadClusterState(); err != nil {
				return err
			}
		} else if err := d.create(); err != nil {
			return err
		}

		if err := run([]func() error{
			d.copyKubeconfig,
			d.setupDisks,
			createStorageClass,
		}); err != nil {
			return err
		}
		if err := createBucketIfConfigured(d.plan, d.newBucketManager); err != nil {
			return err
		}
		return nil
	default:
		return fmt.Errorf("unknown operation %s", d.plan.Operation)
	}
	return nil
}

func (d *OCPDriver) create() error {
	log.Println("Creating cluster...")
	params := map[string]any{
		GoogleCloudProjectCtxKey: d.plan.Ocp.GCloudProject,
		"ClusterName":            d.plan.ClusterName,
		"Region":                 d.plan.Ocp.Region,
		"AdminUsername":          d.plan.Ocp.AdminUsername,
		"KubernetesVersion":      d.plan.KubernetesVersion,
		"MachineType":            d.plan.MachineType,
		"LocalSsdCount":          d.plan.Ocp.LocalSsdCount,
		"NodeCount":              d.plan.Ocp.NodeCount,
		"BaseDomain":             d.baseDomain(),
		"OCPStateBucket":         OCPStateBucket,
		"PullSecret":             d.plan.Ocp.PullSecret,
		"UserLabels":             elasticTags,
	}
	var tpl bytes.Buffer
	if err := template.Must(template.New("").Parse(OcpInstallerConfigTemplate)).Execute(&tpl, params); err != nil {
		return err
	}

	installConfig := filepath.Join(d.runtimeState.ClusterStateDir, "install-config.yaml")
	err := os.WriteFile(installConfig, tpl.Bytes(), 0o600)
	if err != nil {
		return err
	}

	err = d.runInstallerCommand("create")

	// We want to *always* upload the state of the cluster
	// this way we can run a delete operation even on failed
	// deployments to clean all the resources on GCP.
	_ = d.uploadClusterState()
	return err
}

func (d *OCPDriver) delete() error {
	log.Printf("Deleting cluster %s ...\n", d.plan.ClusterName)

	err := d.runInstallerCommand("destroy")
	if err != nil {
		return err
	}

	// No need to check whether this `rm` command succeeds
	_ = exec.NewCommand("gcloud storage rm -r gs://{{.OCPStateBucket}}/{{.ClusterName}}").AsTemplate(d.bucketParams()).WithoutStreaming().Run()
	d.runtimeState.SafeToDeleteWorkdir = true
	// Only remove kubeconfig when it was merged by this deployer (normal create/delete flow).
	// During cleanup the cluster was created by a different CI job, there is no local kubeconfig to remove.
	if d.plan.Ocp.InfraID == "" {
		return d.removeKubeconfig()
	}
	return nil
}

func (d *OCPDriver) GetCredentials() error {
	if err := run(d.setup()); err != nil {
		return err
	}

	defer func() {
		_ = d.removeWorkDir()
	}()

	return d.copyKubeconfig()
}

func run(steps []func() error) error {
	for _, fn := range steps {
		if err := fn(); err != nil {
			return err
		}
	}
	return nil
}

func (d *OCPDriver) setupDisks() error {
	return setupDisks(d.plan)
}

func (d *OCPDriver) ensureClientImage() error {
	image, err := ensureClientImage(OCPDriverID, vault.Provide(d.vaultClient), d.plan.ClientVersion, d.plan.ClientBuildDefDir)
	if err != nil {
		return err
	}
	d.runtimeState.ClientImage = image
	return nil
}

func (d *OCPDriver) ensurePullSecret() error {
	if d.plan.Ocp.PullSecret == "" {
		s, err := vault.Get(d.vaultClient, OCPVaultPath, OCPPullSecretFieldName)
		if err != nil {
			return err
		}
		d.plan.Ocp.PullSecret = s
	}
	return nil
}

func (d *OCPDriver) ensureWorkDir() error {
	if d.runtimeState.ClusterStateDir != "" {
		// already initialised
		return nil
	}
	workDir := d.plan.Ocp.WorkDir
	if workDir == "" {
		// base work dir in HOME dir otherwise mounting to container won't work without further settings adjustment
		// in macOS in local mode. In CI mode we need the workdir to be in the volume shared between containers.
		// having the work dir in HOME also underlines the importance of the work dir contents. The work dir is the only
		// source to cleanly uninstall the cluster should the rsync fail.
		var err error
		workDir, err = os.MkdirTemp(os.Getenv("HOME"), d.plan.ClusterName)
		if err != nil {
			return err
		}
		log.Printf("Defaulting WorkDir: %s", workDir)
	}

	if err := os.MkdirAll(workDir, os.ModePerm); err != nil {
		return err
	}
	d.runtimeState.ClusterStateDir = workDir
	log.Printf("Using ClusterStateDir: %s", workDir)
	return nil
}

func (d *OCPDriver) removeWorkDir() error {
	if !d.runtimeState.SafeToDeleteWorkdir {
		log.Printf("Not deleting work dir as rsync backup of installer state not successful")
		return nil
	}
	// keep workdir around useful for debugging or when running in non-CI mode
	if d.plan.Ocp.StickyWorkDir {
		log.Printf("Not deleting work dir as requested via StickyWorkDir option")
		return nil
	}
	return os.RemoveAll(d.plan.Ocp.WorkDir)
}

func (d *OCPDriver) authToGCP() error {
	// avoid double authentication
	if d.runtimeState.Authenticated {
		return nil
	}

	if err := authToGCP(
		d.vaultClient, OCPVaultPath, OCPServiceAccountVaultFieldName,
		d.plan.ServiceAccount, false, d.plan.Ocp.GCloudProject,
	); err != nil {
		return err
	}
	d.runtimeState.Authenticated = true
	return nil
}

type ClusterStatus string

var (
	PartiallyDeployed ClusterStatus = "PartiallyDeployed"
	NotFound          ClusterStatus = "NotFound"
	NotResponding     ClusterStatus = "NotResponding"
	Running           ClusterStatus = "Running"
)

func (d *OCPDriver) currentStatus() ClusterStatus {
	log.Println("Checking if cluster exists...")

	kubeConfig := filepath.Join(d.runtimeState.ClusterStateDir, "auth", "kubeconfig")
	if _, err := os.Stat(kubeConfig); os.IsNotExist(err) {
		if empty, err := isEmpty(d.runtimeState.ClusterStateDir); empty && err == nil {
			return NotFound
		}
		return PartiallyDeployed
	}

	log.Println("Cluster state synced: Testing that the OpenShift cluster is alive... ")
	cmd := "kubectl version"
	alive, err := exec.NewCommand(cmd).WithoutStreaming().WithVariable("KUBECONFIG", kubeConfig).OutputContainsAny("Server Version")

	if !alive || err != nil { // error will be typically not nil when alive is false but let's be explicit here to avoid returning Running on a non-nil error
		log.Printf("a cluster state dir was found in %s but the cluster is not responding to `kubectl version`: %s", d.runtimeState.ClusterStateDir, err.Error())
		return NotResponding
	}

	return Running
}

func isEmpty(dir string) (bool, error) {
	// https://stackoverflow.com/questions/30697324/how-to-check-if-directory-on-path-is-empty
	f, err := os.Open(dir)
	if err != nil {
		return false, err
	}
	defer f.Close()

	_, err = f.Readdirnames(1)
	if errors.Is(err, io.EOF) {
		return true, nil
	}
	return false, err
}

func (d *OCPDriver) uploadClusterState() error {
	// Let's check that the cluster dir exists
	// before we attempt an upload.
	if _, err := os.Stat(d.runtimeState.ClusterStateDir); os.IsNotExist(err) {
		log.Printf("clusterStateDir %s not present", d.runtimeState.ClusterStateDir)
		return nil
	}

	bucketNotFound, err := exec.NewCommand("gcloud storage ls gs://{{.OCPStateBucket}}").
		AsTemplate(d.bucketParams()).
		WithoutStreaming().
		OutputContainsAny("BucketNotFoundException", "not found: 404")
	if err != nil {
		return fmt.Errorf("while checking state bucket existence %w", err)
	}
	if bucketNotFound {
		if err := exec.NewCommand("gcloud storage buckets create gs://{{.OCPStateBucket}}").AsTemplate(d.bucketParams()).Run(); err != nil {
			return fmt.Errorf("while creating storage bucket: %w", err)
		}
	}

	// rsync seems to get stuck at least in local mode every now and then let's retry a few times
	err = exec.NewCommand("gcloud storage rsync {{.ClusterStateDir}} gs://{{.OCPStateBucket}}/{{.ClusterName}} -r --delete-unmatched-destination-objects").
		WithLog("Uploading cluster state").
		AsTemplate(d.bucketParams()).
		WithoutStreaming().
		RunWithRetries(3, 15*time.Minute)
	if err == nil {
		d.runtimeState.SafeToDeleteWorkdir = true
	}
	return err
}

// downloadClusterState syncs the OCP installer state from the GCS bucket to the local work directory.
// If the remote state does not exist, the error is swallowed and nil is returned, leaving the work
// directory empty. An empty work directory causes currentStatus() to return NotFound — the expected
// state for a new cluster creation or when the state was lost after a failed create.
//
// During cleanup (delete operation) this is problematic: an empty work directory causes Execute to
// skip deletion ("cluster doesn't exist"), leaving orphaned GCP resources behind. As a last resort,
// when the operation is a delete and an InfraID has been set on the plan (only done by Cleanup,
// which extracts it from running instance names), a synthetic metadata.json is written to the work
// directory instead. This is the minimum required for openshift-install destroy to locate and delete
// all GCP resources associated with the infra ID.
func (d *OCPDriver) downloadClusterState() error {
	cmd := "gcloud storage rsync gs://{{.OCPStateBucket}}/{{.ClusterName}} {{.ClusterStateDir}} -r --delete-unmatched-destination-objects"
	doesNotExist, err := exec.NewCommand(cmd).
		AsTemplate(d.bucketParams()).
		WithLog("Synching cluster state").
		WithoutStreaming().
		OutputContainsAny("BucketNotFoundException", "does not name a directory, bucket, or bucket subdir", "Did not find existing container")
	if doesNotExist {
		log.Printf("No remote cluster state found")
		// During cleanup (delete with an infra ID), write a synthetic metadata.json as a
		// last resort so that openshift-install destroy can still locate and delete GCP resources.
		if d.plan.Operation == DeleteAction && d.plan.Ocp.InfraID != "" {
			log.Printf("Infra ID %s available, generating synthetic metadata.json as fallback", d.plan.Ocp.InfraID)
			return d.writeMetadataJSON()
		}
		return nil // swallow this error as it is expected if no cluster has been created yet
	}
	return err
}

func (d *OCPDriver) copyKubeconfig() error {
	log.Printf("Copying credentials")
	kubeConfig := filepath.Join(d.runtimeState.ClusterStateDir, "auth", "kubeconfig")

	// 1. merge or create kubeconfig
	if err := mergeKubeconfig(kubeConfig); err != nil {
		return err
	}
	// 2. after merging make sure that the ocp context is in use, which is always called `admin`
	return exec.NewCommand("kubectl config use-context admin").Run()
}

func (d *OCPDriver) removeKubeconfig() error {
	return removeKubeconfig("admin", "admin", "admin")
}

func (d *OCPDriver) bucketParams() map[string]any {
	return map[string]any{
		"OCPStateBucket":  OCPStateBucket,
		"ClusterName":     d.plan.ClusterName,
		"ClusterStateDir": d.runtimeState.ClusterStateDir,
	}
}

func (d *OCPDriver) runInstallerCommand(action string) error {
	// Use warn level for create (cluster credentials are logged at info level),
	// info level for destroy (useful to see which resources are being deleted).
	logLevel := "warn"
	if action == "destroy" {
		logLevel = "info"
	}
	params := map[string]any{
		"ClusterStateDirBase": filepath.Base(d.runtimeState.ClusterStateDir),
		"SharedVolume":        env.SharedVolumeName(),
		"GCloudCredsPath":     filepath.Join("/home", GCPDir, ServiceAccountFilename),
		"OCPToolsDockerImage": d.runtimeState.ClientImage,
		"Action":              action,
		"LogLevel":            logLevel,
	}
	// We are mounting the shared volume into the installer container and configure it to be the HOME directory
	// this is mainly so that the GCloud tooling picks up the authentication information correctly as the base image is
	// scratch+curl and thus an empty
	// We are mounting tmp as the installer needs a scratch space and writing into the container won't work
	cmd := exec.NewCommand(`docker run --rm \
		-v {{.SharedVolume}}:/home \
		-v /tmp:/tmp \
		-e GOOGLE_APPLICATION_CREDENTIALS={{.GCloudCredsPath}} \
		-e HOME=/home \
		{{.OCPToolsDockerImage}} \
		/openshift-install {{.Action}} cluster --log-level {{.LogLevel}} --dir /home/{{.ClusterStateDirBase}}`)
	return cmd.AsTemplate(params).Run()
}

func (d *OCPDriver) baseDomain() string {
	baseDomain := d.plan.Ocp.BaseDomain
	// Domains used for the OCP deployment must be
	// pre-configured on the destination cloud. A zone
	// for these domains must exist and it has to be
	// reachable from the internet as `openshift-installer`
	// interacts with the deployed OCP cluster to monitor
	// and complete the deployment.
	//
	// The default `eck-ocp.elastic.dev` subdomain is configured
	// on AWS as an NS record and points to a zone configured in
	// the `elastic-cloud-dev` project on GCP.
	if baseDomain == "" {
		baseDomain = "eck-ocp.elastic.dev"
	}
	return baseDomain
}

func (d *OCPDriver) newBucketManager() (bucket.Manager, error) {
	// Use VaultManager for pre-provisioned buckets
	if d.plan.Bucket.FromVault {
		return newVaultBucketManager(OCPDriverID, d.vaultClient)
	}

	// Use GCSManager for dynamic bucket creation
	if err := bucket.ValidateShellArg(d.plan.Ocp.GCloudProject, "GCP project"); err != nil {
		return nil, err
	}
	if d.plan.Bucket.StorageClass != "" {
		if err := bucket.ValidateShellArg(d.plan.Bucket.StorageClass, "storage class"); err != nil {
			return nil, err
		}
	}
	ctx := map[string]any{
		"ClusterName": d.plan.ClusterName,
		"PlanId":      d.plan.Id,
	}
	cfg, err := newBucketConfig(d.plan, ctx, d.plan.Ocp.Region)
	if err != nil {
		return nil, err
	}
	return bucket.NewGCSManager(cfg, d.plan.Ocp.GCloudProject, d.plan.Bucket.StorageClass), nil
}

func (d *OCPDriver) Cleanup(prefix string, olderThan time.Duration) error {
	if err := d.authToGCP(); err != nil {
		return err
	}
	sinceDate := time.Now().Add(-olderThan)

	params := d.bucketParams()
	params["Date"] = sinceDate.Format(time.RFC3339)
	params["E2EClusterNamePrefix"] = prefix
	params["Region"] = d.plan.Ocp.Region

	if d.plan.Ocp.GCloudProject == "" {
		gCloudProject, err := vault.Get(d.vaultClient, OCPVaultPath, GKEProjectVaultFieldName)
		if err != nil {
			return err
		}
		d.plan.Ocp.GCloudProject = gCloudProject
	}
	params["GCloudProject"] = d.plan.Ocp.GCloudProject

	zonesCmd := `gcloud compute zones list --verbosity error ` +
		`--filter='region:https://www.googleapis.com/compute/v1/projects/{{.GCloudProject}}/regions/{{.Region}}' ` +
		`--format="value(selfLink.name())"`
	zones, err := exec.NewCommand(zonesCmd).AsTemplate(params).WithoutStreaming().OutputList()
	if err != nil {
		return err
	}
	params["Zones"] = strings.Join(zones, ",")

	// Discover orphaned infra IDs from multiple GCP resource types.
	// Each source returns infra IDs filtered by the cutoff date.
	fromInstances, err := listInfraIDsFromInstances(params)
	if err != nil {
		return err
	}
	fromNetworks, err := listInfraIDsFromNetworks(params)
	if err != nil {
		return err
	}
	fromSAs, err := listInfraIDsFromServiceAccounts(params)
	if err != nil {
		return err
	}

	// Merge all discovered infra IDs, deduplicating.
	allInfraIDs := set.Make(slices.Concat(fromInstances, fromNetworks, fromSAs)...)
	if allInfraIDs.Count() == 0 {
		log.Printf("No orphaned OCP clusters found")
		return nil
	}
	infraIDSlice := allInfraIDs.AsSlice()
	log.Printf("Found %d orphaned infra ID(s) to clean up: %v", len(infraIDSlice), infraIDSlice)

	// Map each infra ID back to a cluster name and run destroy.
	// The infra ID is {clusterName}-{5alphanumChars}. We extract the cluster name by matching
	// the full infra ID pattern and capturing everything before the 5-char suffix. Internal OCP
	// component SAs (e.g. eck-e2e-ocp--cloud-crede-lhtvg) are filtered out because the regex
	// requires at least one alphanumeric char between each dash ([a-z0-9]+), rejecting the
	// double dash ("--") present in their names.
	infraIDRE := buildInfraIDRegexp(prefix)
	var deleted, failed, skipped int
	for _, infraID := range infraIDSlice {
		matches := infraIDRE.FindStringSubmatch(infraID)
		if len(matches) < 2 {
			log.Printf("Skipping %s: internal OCP component, will be cleaned up with its parent cluster", infraID)
			skipped++
			continue
		}
		clusterName := matches[1]
		d.plan.ClusterName = clusterName
		d.plan.Ocp.InfraID = infraID
		d.plan.Operation = DeleteAction
		if err = d.Execute(); err != nil {
			log.Printf("while deleting cluster %s (infra ID %s): %v", clusterName, infraID, err)
			failed++
			continue
		}
		deleted++
	}
	log.Printf("Cleanup complete: %d deleted, %d failed, %d skipped", deleted, failed, skipped)
	return nil
}

// listInfraIDsFromInstances returns infra IDs extracted from running GCP instances
// that are older than the cutoff date and match the cluster name prefix.
func listInfraIDsFromInstances(params map[string]any) ([]string, error) {
	cmd := `gcloud compute instances list --verbosity error ` +
		`--zones={{.Zones}} ` +
		`--filter="name~'^{{.E2EClusterNamePrefix}}-ocp' AND status=RUNNING AND creationTimestamp<='{{.Date}}'" ` +
		`--format="value(name)" ` +
		`| sed -E 's/-(master-[0-9]+|bootstrap|worker-.*)$//' | sort | uniq`
	return exec.NewCommand(cmd).AsTemplate(params).WithoutStreaming().OutputList()
}

// listInfraIDsFromNetworks returns infra IDs extracted from GCP networks
// that are older than the cutoff date and match the cluster name prefix.
// Networks are created early during OCP installation and deleted last,
// making them a reliable signal for orphaned clusters.
func listInfraIDsFromNetworks(params map[string]any) ([]string, error) {
	cmd := `gcloud compute networks list --verbosity error ` +
		`--filter="name~'^{{.E2EClusterNamePrefix}}-ocp' AND creationTimestamp<='{{.Date}}'" ` +
		`--format="value(name)" ` +
		`| sed 's/-network$//' | sort | uniq`
	return exec.NewCommand(cmd).AsTemplate(params).WithoutStreaming().OutputList()
}

// listInfraIDsFromServiceAccounts returns infra IDs extracted from GCP service accounts
// that match the cluster name prefix and have at least one key older than the cutoff date.
// Service accounts don't expose a creation timestamp in the IAM API, so we use the oldest
// key's validAfterTime as a proxy.
func listInfraIDsFromServiceAccounts(params map[string]any) ([]string, error) {
	// List all service accounts matching the prefix.
	listCmd := `gcloud iam service-accounts list --verbosity error ` +
		`--filter="email~'{{.E2EClusterNamePrefix}}-ocp'" ` +
		`--format="value(email)" ` +
		`--project={{.GCloudProject}}`
	emails, err := exec.NewCommand(listCmd).AsTemplate(params).WithoutStreaming().OutputList()
	if err != nil {
		return nil, err
	}

	// Multiple SAs share the same infra ID (master, worker, bootstrap). We only need to check
	// keys for one of them since they are all created at the same time.
	seen := set.Make()
	for i, email := range emails {
		matches := ocpServiceAccountRE.FindStringSubmatch(email)
		if len(matches) < 2 {
			log.Printf("warning: could not extract infra ID from service account %s", email)
			continue
		}
		infraID := matches[1]

		if seen.Has(infraID) {
			continue
		}

		log.Printf("Checking keys for service account %s (%d/%d)", email, i+1, len(emails))
		keysCmd := fmt.Sprintf(
			`gcloud iam service-accounts keys list --verbosity error `+
				`--iam-account=%s `+
				`--filter="validAfterTime<='{{.Date}}'" `+
				`--format="value(name)" `+
				`--managed-by=user `+
				`--project={{.GCloudProject}}`,
			email,
		)
		keys, err := exec.NewCommand(keysCmd).AsTemplate(params).WithoutStreaming().OutputList()
		if err != nil {
			log.Printf("warning: could not list keys for %s: %v", email, err)
			continue
		}
		if len(keys) > 0 {
			seen.Add(infraID)
		}
	}
	return seen.AsSlice(), nil
}

// buildInfraIDRegexp returns a compiled regexp that matches an OCP infra ID and captures
// the cluster name in group 1. The infra ID format is {clusterName}-{5alphanumChars}.
func buildInfraIDRegexp(prefix string) *regexp.Regexp {
	return regexp.MustCompile(`^(` + regexp.QuoteMeta(prefix) + `-ocp-[a-z0-9]+(?:-[a-z0-9]+)*)-[a-z0-9]{5}$`)
}

// ocpMetadata is the minimal metadata.json structure required by openshift-install destroy.
type ocpMetadata struct {
	ClusterName string `json:"clusterName"`
	// ClusterID is a UUID assigned during creation. It is only used for telemetry, not for
	// resource lookup, so it is left empty in synthetic metadata. The field is kept (without
	// omitempty) to match the schema that openshift-install expects.
	ClusterID string       `json:"clusterID"`
	InfraID   string       `json:"infraID"`
	GCP       *gcpMetadata `json:"gcp,omitempty"`
}

type gcpMetadata struct {
	Region    string `json:"region"`
	ProjectID string `json:"projectID"`
}

// writeMetadataJSON creates a synthetic metadata.json in the cluster state directory
// so that openshift-install destroy can locate and delete GCP resources by infra ID,
// even when the original installer state is missing from the GCS bucket.
func (d *OCPDriver) writeMetadataJSON() error {
	meta := ocpMetadata{
		ClusterName: d.plan.ClusterName,
		InfraID:     d.plan.Ocp.InfraID,
		GCP: &gcpMetadata{
			Region:    d.plan.Ocp.Region,
			ProjectID: d.plan.Ocp.GCloudProject,
		},
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("while marshalling metadata.json: %w", err)
	}
	metadataPath := filepath.Join(d.runtimeState.ClusterStateDir, "metadata.json")
	log.Printf("Writing synthetic metadata.json for infra ID %s to %s", d.plan.Ocp.InfraID, metadataPath)
	return os.WriteFile(metadataPath, data, 0600)
}
