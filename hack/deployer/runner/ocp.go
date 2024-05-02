// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package runner

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/elastic/cloud-on-k8s/v2/hack/deployer/exec"
	"github.com/elastic/cloud-on-k8s/v2/hack/deployer/runner/env"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/vault"
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
pullSecret: '{{.PullSecret}}'`
)

func init() {
	drivers[OCPDriverID] = &OCPDriverFactory{}
}

type OCPDriverFactory struct {
}

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
		if clusterStatus != NotFound {
			// always attempt a deletion
			return d.delete()
		}
		log.Printf("Not deleting as cluster doesn't exist")
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

		return run([]func() error{
			d.copyKubeconfig,
			d.setupDisks,
			createStorageClass,
		})
	default:
		return fmt.Errorf("unknown operation %s", d.plan.Operation)
	}
	return nil
}

func (d *OCPDriver) create() error {
	log.Println("Creating cluster...")
	params := map[string]interface{}{
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
	}
	var tpl bytes.Buffer
	if err := template.Must(template.New("").Parse(OcpInstallerConfigTemplate)).Execute(&tpl, params); err != nil {
		return err
	}

	installConfig := filepath.Join(d.runtimeState.ClusterStateDir, "install-config.yaml")
	err := os.WriteFile(installConfig, tpl.Bytes(), 0600)
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
	_ = exec.NewCommand("gsutil rm -r gs://{{.OCPStateBucket}}/{{.ClusterName}}").AsTemplate(d.bucketParams()).WithoutStreaming().Run()
	d.runtimeState.SafeToDeleteWorkdir = true
	return d.removeKubeconfig()
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
	image, err := ensureClientImage(OCPDriverID, d.vaultClient, d.plan.ClientVersion, d.plan.ClientBuildDefDir)
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

	bucketNotFound, err := exec.NewCommand("gsutil ls gs://{{.OCPStateBucket}}").
		AsTemplate(d.bucketParams()).
		WithoutStreaming().
		OutputContainsAny("BucketNotFoundException")
	if err != nil {
		return fmt.Errorf("while checking state bucket existence %w", err)
	}
	if bucketNotFound {
		if err := exec.NewCommand("gsutil mb gs://{{.OCPStateBucket}}").AsTemplate(d.bucketParams()).Run(); err != nil {
			return fmt.Errorf("while creating storage bucket: %w", err)
		}
	}

	// rsync seems to get stuck at least in local mode every now and then let's retry a few times
	err = exec.NewCommand("gsutil rsync -r -d {{.ClusterStateDir}} gs://{{.OCPStateBucket}}/{{.ClusterName}}").
		WithLog("Uploading cluster state").
		AsTemplate(d.bucketParams()).
		WithoutStreaming().
		RunWithRetries(3, 15*time.Minute)
	if err == nil {
		d.runtimeState.SafeToDeleteWorkdir = true
	}
	return err
}

func (d *OCPDriver) downloadClusterState() error {
	cmd := "gsutil rsync -r -d gs://{{.OCPStateBucket}}/{{.ClusterName}} {{.ClusterStateDir}}"
	doesNotExist, err := exec.NewCommand(cmd).
		AsTemplate(d.bucketParams()).
		WithLog("Synching cluster state").
		WithoutStreaming().
		OutputContainsAny("BucketNotFoundException", "does not name a directory, bucket, or bucket subdir")
	if doesNotExist {
		log.Printf("No remote cluster state found")
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

func (d *OCPDriver) bucketParams() map[string]interface{} {
	return map[string]interface{}{
		"OCPStateBucket":  OCPStateBucket,
		"ClusterName":     d.plan.ClusterName,
		"ClusterStateDir": d.runtimeState.ClusterStateDir,
	}
}

func (d *OCPDriver) runInstallerCommand(action string) error {
	params := map[string]interface{}{
		"ClusterStateDirBase": filepath.Base(d.runtimeState.ClusterStateDir),
		"SharedVolume":        env.SharedVolumeName(),
		"GCloudCredsPath":     filepath.Join("/home", GCPDir, ServiceAccountFilename),
		"OCPToolsDockerImage": d.runtimeState.ClientImage,
		"Action":              action,
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
		/openshift-install {{.Action}} cluster --log-level warn --dir /home/{{.ClusterStateDirBase}}`)
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

	zonesCmd := `gcloud compute zones list --verbosity error --filter='region:https://www.googleapis.com/compute/v1/projects/{{.GCloudProject}}/regions/{{.Region}}' --format="value(selfLink.name())"`
	zones, err := exec.NewCommand(zonesCmd).AsTemplate(params).WithoutStreaming().OutputList()
	if err != nil {
		return err
	}
	params["Zones"] = strings.Join(zones, ",")

	cmd := `gcloud compute instances list --verbosity error --zones={{.Zones}} --filter="name~'^{{.E2EClusterNamePrefix}}-ocp.*' AND status=RUNNING" --format=json | jq -r --arg d "{{.Date}}" 'map(select(.creationTimestamp | . <= $d))|.[].name' | grep -o '{{.E2EClusterNamePrefix}}-ocp-[a-z]*-[0-9]*' | sort | uniq`
	clustersToDelete, err := exec.NewCommand(cmd).AsTemplate(params).WithoutStreaming().OutputList()
	if err != nil {
		return err
	}

	for _, cluster := range clustersToDelete {
		d.plan.ClusterName = cluster
		d.plan.Operation = DeleteAction
		if err = d.Execute(); err != nil {
			log.Printf("while deleting cluster %s: %v", cluster, err.Error())
			continue
		}
	}
	return nil
}
