// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package runner

import (
	"errors"
	"fmt"
	"log"
	"maps"
	"strings"
	"time"

	"github.com/ghodss/yaml"
	storagev1 "k8s.io/api/storage/v1"

	"github.com/elastic/cloud-on-k8s/v3/hack/deployer/exec"
	"github.com/elastic/cloud-on-k8s/v3/hack/deployer/runner/bucket"
	"github.com/elastic/cloud-on-k8s/v3/hack/deployer/runner/kyverno"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/retry"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/vault"
)

const (
	storageClassPrefix              = "e2e-"
	GKEDriverID                     = "gke"
	GKEVaultPath                    = "ci-gcp-k8s-operator"
	GKEServiceAccountVaultFieldName = "service-account"
	GKEProjectVaultFieldName        = "gcloud-project"
	GoogleCloudProjectCtxKey        = "GCloudProject"
	gkeLocalDiskSetupCommand        = "kubectl apply -k hack/deployer/config/local-disks-gke"
	DefaultGKERunConfigTemplate     = `id: gke-dev
overrides:
  clusterName: %s-dev-cluster
  gke:
    gCloudProject: %s
`
	gkeCapacityErrorIndicator = "does not have enough resources available to fulfill request"
	gkeQuotaErrorIndicator    = "insufficient quota to satisfy the request"
)

var (
	// GKE uses 18 chars to prefix the pvc created by a cluster
	pvcPrefixMaxLength      = 18
	defaultClusterIPv4CIDR  = "/20"
	defaultServicesIPv4CIDR = "/20"
)

func init() {
	drivers[GKEDriverID] = &GKEDriverFactory{}
}

type GKEDriverFactory struct {
}

type GKEDriver struct {
	plan        Plan
	clusters    []clusterContext
	vaultClient vault.Client
	// overrides for testing
	newBucketManagerFn   func(Plan, clusterContext) (bucket.Manager, error)
	createStorageClassFn func() error
}

type clusterContext interface {
	region() string
	values() map[string]any
	exists() (bool, error)
	create(Plan) (string, error)
	updateLabels() error
	delete() error
	bindRoles(Plan) error
	copyBuiltInStorageClasses() error
	getCredentials() error
	listClusters(string, time.Time) ([]string, error)
	withClusterName(string) clusterContext
	withProject(string) clusterContext
}

type gkeClusterContext map[string]any

func (c gkeClusterContext) region() string {
	region, _ := c["Region"].(string)
	return region
}

func (c gkeClusterContext) values() map[string]any {
	return c
}

func (c gkeClusterContext) clone() gkeClusterContext {
	cloned := make(gkeClusterContext, len(c))
	maps.Copy(cloned, c)
	return cloned
}

func (c gkeClusterContext) withClusterName(name string) clusterContext {
	cloned := c.clone()
	cloned["ClusterName"] = name
	return cloned
}

func (c gkeClusterContext) withProject(project string) clusterContext {
	cloned := c.clone()
	cloned[GoogleCloudProjectCtxKey] = project
	return cloned
}

func (gdf *GKEDriverFactory) Create(plan Plan) (Driver, error) {
	localSSDOption, err := gkeLocalSSDOption(plan.Gke)
	if err != nil {
		return nil, err
	}
	plan = configureGKELocalSSD(plan)

	pvcPrefix := plan.ClusterName
	if len(pvcPrefix) > pvcPrefixMaxLength {
		pvcPrefix = pvcPrefix[0:pvcPrefixMaxLength]
	}

	clusterIPv4CIDR := defaultClusterIPv4CIDR
	if plan.Gke.ClusterIPv4CIDR != "" {
		clusterIPv4CIDR = plan.Gke.ClusterIPv4CIDR
	}

	servicesIPv4CIDR := defaultServicesIPv4CIDR
	if plan.Gke.ServicesIPv4CIDR != "" {
		servicesIPv4CIDR = plan.Gke.ServicesIPv4CIDR
	}

	c, err := vault.NewClient()
	if err != nil {
		return nil, err
	}

	regions := gkeRegions(plan.Gke.Region, plan.Gke.FallbackRegions)
	clusters := make([]clusterContext, 0, len(regions))
	for _, region := range regions {
		clusters = append(clusters, gkeClusterContext{
			GoogleCloudProjectCtxKey: plan.Gke.GCloudProject,
			"ClusterName":            plan.ClusterName,
			"PVCPrefix":              pvcPrefix,
			"PlanId":                 plan.Id,
			"Region":                 region,
			"KubernetesVersion":      plan.KubernetesVersion,
			"MachineType":            plan.MachineType,
			"LocalSSDOption":         localSSDOption,
			"GcpScopes":              plan.Gke.GcpScopes,
			"NodeCountPerZone":       plan.Gke.NodeCountPerZone,
			"ClusterIPv4CIDR":        clusterIPv4CIDR,
			"ServicesIPv4CIDR":       servicesIPv4CIDR,
		})
	}

	return &GKEDriver{
		plan:        plan,
		clusters:    clusters,
		vaultClient: c,
	}, nil
}

func gkeRegions(primary string, fallbacks []string) []string {
	seen := make(map[string]struct{}, len(fallbacks)+1)
	regions := make([]string, 0, len(fallbacks)+1)

	seen[primary] = struct{}{}
	regions = append(regions, primary)

	for _, region := range fallbacks {
		if _, exists := seen[region]; exists {
			continue
		}
		seen[region] = struct{}{}
		regions = append(regions, region)
	}
	return regions
}

func gkeLocalSSDOption(settings *GKESettings) (string, error) {
	if settings == nil {
		return "", fmt.Errorf("GKE settings must be configured")
	}
	if settings.LocalSsdCount < 0 {
		return "", fmt.Errorf("local SSD count must not be negative")
	}
	if settings.LocalSsdCount > 0 && settings.LocalNvmeSsdBlock {
		return "", fmt.Errorf("localSsdCount and localNvmeSsdBlock are mutually exclusive")
	}
	if settings.LocalNvmeSsdBlock {
		return "--local-nvme-ssd-block count=1", nil
	}
	if settings.LocalSsdCount > 0 {
		// Deprecated compatibility path for existing GKE deployer overrides.
		log.Printf("WARNING: gke.localSsdCount is deprecated; use gke.localNvmeSsdBlock instead")
		return fmt.Sprintf("--local-ssd-count %d", settings.LocalSsdCount), nil
	}
	return "", nil
}

func configureGKELocalSSD(plan Plan) Plan {
	if plan.Gke.LocalNvmeSsdBlock && plan.DiskSetup == "" {
		plan.DiskSetup = gkeLocalDiskSetupCommand
	}
	return plan
}

func (d *GKEDriver) Execute() error {
	if err := authToGCP(
		d.vaultClient, GKEVaultPath, GKEServiceAccountVaultFieldName,
		d.plan.ServiceAccount, false, d.plan.Gke.GCloudProject,
	); err != nil {
		return err
	}

	switch d.plan.Operation {
	case DeleteAction:
		return d.delete()
	case CreateAction:
		return d.create()
	default:
		return fmt.Errorf("unknown operation %s", d.plan.Operation)
	}
}

func (d *GKEDriver) create() error {
	// A previous run may have created the cluster in any configured region.
	existing, err := findExistingCluster(d.clusters)
	if err != nil {
		return err
	}
	if existing != nil {
		log.Printf("not creating as cluster exists in region %s", existing.region())
		return d.finishCreate(existing, false)
	}

	cCtx, err := createInRegions(d.plan, d.clusters)
	if err != nil {
		return err
	}
	return d.finishCreate(cCtx, true)
}

func findExistingCluster(clusters []clusterContext) (clusterContext, error) {
	for _, cCtx := range clusters {
		exists, err := cCtx.exists()
		if err != nil {
			return nil, fmt.Errorf("check cluster in region %s: %w", cCtx.region(), err)
		}
		if exists {
			return cCtx, nil
		}
	}
	return nil, nil
}

func createInRegions(plan Plan, clusters []clusterContext) (clusterContext, error) {
	if len(clusters) == 0 {
		return nil, errors.New("no GKE regions configured")
	}

	var errs error
	for _, cCtx := range clusters {
		output, err := cCtx.create(plan)
		if err != nil {
			region := cCtx.region()
			createErr := fmt.Errorf("create cluster in region %s: %w", region, err)
			if deleteErr := cCtx.delete(); deleteErr != nil {
				return nil, errors.Join(errs, createErr, fmt.Errorf("clean up failed creation in region %s: %w", region, deleteErr))
			}
			if !isCapacityError(output) {
				return nil, errors.Join(errs, createErr)
			}
			errs = errors.Join(errs, createErr)
			continue
		}
		return cCtx, nil
	}
	return nil, errs
}

func isCapacityError(output string) bool {
	return strings.Contains(output, gkeCapacityErrorIndicator) ||
		strings.Contains(output, gkeQuotaErrorIndicator)
}

func (d *GKEDriver) finishCreate(cCtx clusterContext, created bool) error {
	// Existing clusters skip label updates and role binding. A previous partial run
	// may therefore leave an existing cluster under-configured; this is a known limitation.
	if created {
		if err := d.configureCreatedCluster(cCtx); err != nil {
			return err
		}
	} else {
		log.Printf("warning: existing cluster found; skipping label and role setup; cluster may be under-configured if a previous run was interrupted")
	}

	if d.plan.Gke.Private {
		log.Printf("a private cluster has been created, please retrieve credentials manually and create storage class and provider if needed")
		log.Printf("to authorize a VM to access this cluster run the following command:\n"+
			"$ gcloud container clusters update %s"+
			" --region %s "+
			"--enable-master-authorized-networks"+
			" --master-authorized-networks  <VM IP>/32",
			d.plan.ClusterName, cCtx.region())
		log.Printf("you can then retrieve the credentials with the following command:\n"+
			"$ gcloud container clusters get-credentials %s"+
			" --region %s "+
			" --project %s",
			d.plan.ClusterName, cCtx.region(), d.plan.Gke.GCloudProject)
		return nil
	}

	if err := cCtx.getCredentials(); err != nil {
		return err
	}

	if err := cCtx.copyBuiltInStorageClasses(); err != nil {
		return err
	}

	if err := setupDisks(d.plan); err != nil {
		return err
	}
	createStorageClassFn := createStorageClass
	if d.createStorageClassFn != nil {
		createStorageClassFn = d.createStorageClassFn
	}
	if err := createStorageClassFn(); err != nil {
		return err
	}
	if d.plan.EnforceSecurityPolicies {
		if err := kyverno.Install(); err != nil {
			return err
		}
		// apply extra policies to prevent use of unlabeled storage classes which might escape garbage collection in CI.
		// Retry because `rollout status` (used in kyverno.Install) only guarantees the pods are available,
		// not that the webhook server is ready to accept connections yet.
		if err := retry.UntilSuccess(
			func() error { return apply(kyverno.GKEPolicies) },
			2*time.Minute,
			5*time.Second,
		); err != nil {
			return err
		}
	}
	if err := createBucketIfConfigured(d.plan, d.bucketManagerFactory(d.plan, cCtx)); err != nil {
		return err
	}
	return nil
}

func (d *GKEDriver) configureCreatedCluster(cCtx clusterContext) error {
	// Gcloud does not support labels when creating Autopilot clusters.
	if d.plan.Gke.Autopilot {
		if err := cCtx.updateLabels(); err != nil {
			return err
		}
	}
	return cCtx.bindRoles(d.plan)
}

func (d *GKEDriver) delete() error {
	var errs error
	for _, cCtx := range d.clusters {
		// Try bucket cleanup for every configured region because bucket templates may
		// include Region and the cluster may already have been deleted. The GCS manager
		// used here is idempotent, so repeated deletion of the same bucket is safe.
		if err := deleteBucketIfConfigured(d.plan, d.bucketManagerFactory(d.plan, cCtx)); err != nil {
			log.Printf("warning: bucket deletion failed, will continue with cluster deletion: %v", err)
			errs = errors.Join(errs, fmt.Errorf("delete bucket in region %s: %w", cCtx.region(), err))
		}
		if err := cCtx.delete(); err != nil {
			log.Printf("warning: cluster deletion failed in region %s, will continue: %v", cCtx.region(), err)
			errs = errors.Join(errs, fmt.Errorf("delete cluster in region %s: %w", cCtx.region(), err))
		}
	}
	return errs
}

const (
	GoogleComputeEngineStorageProvider = "pd.csi.storage.gke.io"
)

// copyBuiltInStorageClasses adds the "labels" parameter to copies of the built-in  GCE storage classes.
// These labels are automatically applied to GCE Persistent Disks provisioned using these storage classes.
func (c gkeClusterContext) copyBuiltInStorageClasses() error {
	storageClassesYaml, err := exec.NewCommand("kubectl get sc -o yaml").WithoutStreaming().Output()
	if err != nil {
		return err
	}
	storageClasses := storagev1.StorageClassList{}
	if err := yaml.Unmarshal([]byte(storageClassesYaml), &storageClasses); err != nil {
		return err
	}

	existingClassNames := make(map[string]struct{})
	for _, sc := range storageClasses.Items {
		existingClassNames[sc.Name] = struct{}{}
	}

	labels, err := c.resourcesLabels()
	if err != nil {
		return err
	}
	for _, storageClass := range storageClasses.Items {
		if storageClass.Provisioner != GoogleComputeEngineStorageProvider {
			continue
		}
		// this function might be called repeatedly
		if strings.HasPrefix(storageClass.Name, storageClassPrefix) {
			continue
		}

		// This is a GCE storage class copy it
		copied := copyWithPrefixAndLabels(storageClass, labels)
		storageClassYaml, err := yaml.Marshal(copied)
		if err != nil {
			return err
		}

		if _, exists := existingClassNames[copied.Name]; exists {
			// do not try to update existing classes
			continue
		}

		// start by removing the default storage class marker so that our copy can take over that role
		if err := exec.NewCommand(fmt.Sprintf(`kubectl annotate sc %s storageclass.kubernetes.io/is-default-class=false --overwrite=true`, storageClass.Name)).
			WithoutStreaming().
			Run(); err != nil {
			return fmt.Errorf("while updating default storage class label: %w", err)
		}
		// kubectl apply the copied storage class
		if err := apply(string(storageClassYaml)); err != nil {
			return err
		}
	}
	return nil
}

func apply(yaml string) error {
	return exec.NewCommand(fmt.Sprintf(`cat <<EOF | kubectl apply -f -
%s
EOF`, yaml)).Run()
}

func copyWithPrefixAndLabels(sc storagev1.StorageClass, labels string) storagev1.StorageClass {
	copied := sc
	// create a new object
	copied.UID = ""
	copied.ResourceVersion = ""
	// remove the addonmanager label from GKE
	delete(copied.Labels, "addonmanager.kubernetes.io/mode")
	// add a prefix to distinguish them from the originals
	copied.Name = storageClassPrefix + sc.Name
	// add the labels for cost attribution and garbage collection
	if copied.Parameters == nil {
		copied.Parameters = make(map[string]string)
	}
	copied.Parameters["labels"] = labels
	return copied
}

func (c gkeClusterContext) resourcesLabels() (string, error) {
	username, err := c.username(true)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(
		"username=%s,cluster_name=%s,plan_id=%s,region=%s",
		username, c["ClusterName"], c["PlanId"], c["Region"],
	), nil
}

func (c gkeClusterContext) fullLabels() (string, error) {
	labels, err := c.resourcesLabels()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s,%s", strings.Join(toList(elasticTags), ","), labels), nil
}

func (c gkeClusterContext) exists() (bool, error) {
	log.Println("Checking if cluster exists...")

	cmd := "gcloud container clusters --project {{.GCloudProject}} describe {{.ClusterName}} --region {{.Region}}"
	contains, err := exec.NewCommand(cmd).AsTemplate(c).WithoutStreaming().OutputContainsAny("Not found")
	if contains {
		return false, nil
	}

	return err == nil, err
}

func (c gkeClusterContext) listClusters(prefix string, before time.Time) ([]string, error) {
	listCtx := c.clone()
	listCtx["Date"] = before.Format(time.RFC3339)
	listCtx["E2EClusterNamePrefix"] = prefix
	cmd := `gcloud container clusters list --verbosity error --region={{.Region}} --format="value(name)" --filter="createTime<{{.Date}} AND name~{{.E2EClusterNamePrefix}}.*"`
	return exec.NewCommand(cmd).AsTemplate(listCtx).OutputList()
}

func (c gkeClusterContext) create(plan Plan) (string, error) {
	log.Println("Creating cluster...")

	var opts []string

	if plan.Gke.NetworkPolicy {
		if plan.Gke.Autopilot {
			return "", fmt.Errorf("--enable-network-policy must not be set if autopilot is enabled")
		}
		opts = append(opts, "--enable-network-policy")
	}

	if plan.Gke.Private {
		opts = append(opts, "--create-subnetwork name={{.ClusterName}}-private-subnet", "--enable-master-authorized-networks", "--enable-ip-alias", "--enable-private-nodes", "--enable-private-endpoint", "--master-ipv4-cidr", "172.16.0.32/28")
	} else {
		opts = append(opts, "--create-subnetwork range={{.ClusterIPv4CIDR}}", "--cluster-ipv4-cidr={{.ClusterIPv4CIDR}}", "--services-ipv4-cidr={{.ServicesIPv4CIDR}}")
	}

	labels, err := c.fullLabels()
	if err != nil {
		return "", err
	}

	var createGKEClusterCommand string
	if !plan.Gke.Autopilot {
		createGKEClusterCommand = `gcloud container --quiet --project {{.GCloudProject}} clusters create {{.ClusterName}} ` +
			`--labels "` + labels + `" --region {{.Region}} --no-enable-basic-auth --cluster-version {{.KubernetesVersion}} ` +
			`--machine-type {{.MachineType}} --disk-type pd-ssd --disk-size 100 ` +
			`{{.LocalSSDOption}} --scopes {{.GcpScopes}} --num-nodes {{.NodeCountPerZone}} ` +
			`--addons HorizontalPodAutoscaling,HttpLoadBalancing ` +
			`--no-enable-autoupgrade --no-enable-autorepair --enable-ip-alias --metadata disable-legacy-endpoints=true ` +
			`--network projects/{{.GCloudProject}}/global/networks/default ` +
			strings.Join(opts, " ")
	} else {
		// Autopilot cluster.
		log.Println("autopilot cluster enabled")
		createGKEClusterCommand = `gcloud container --quiet --project {{.GCloudProject}} clusters create-auto {{.ClusterName}} ` +
			`--region {{.Region}} --cluster-version {{.KubernetesVersion}} ` +
			`--scopes {{.GcpScopes}} --network projects/{{.GCloudProject}}/global/networks/default ` +
			strings.Join(opts, " ")
	}

	output, err := exec.NewCommand(createGKEClusterCommand).
		AsTemplate(c).
		Output()

	if err != nil {
		return output, err
	}

	return output, nil
}

func (c gkeClusterContext) updateLabels() error {
	labels, err := c.fullLabels()
	if err != nil {
		return err
	}
	return exec.NewCommand(`gcloud container --quiet --project {{.GCloudProject}} clusters update {{.ClusterName}} --region {{.Region}} --update-labels="` + labels + `"`).
		AsTemplate(c).
		Run()
}

// username attempts to extract the username from the current account.
// When used in labels the "unqualified" parameter should be set to true, it's because only lowercase letters ([a-z]),
// numeric characters ([0-9]), underscores (_) and dashes (-) are allowed as label values.
func (c gkeClusterContext) username(unqualified bool) (string, error) {
	user, err := exec.NewCommand(`gcloud auth list --filter=status:ACTIVE --format="value(account)"`).WithoutStreaming().Output()
	if err != nil {
		return "", err
	}
	if unqualified {
		if idx := strings.Index(user, "@"); idx != -1 {
			user = user[:idx]
		}
		user = strings.ReplaceAll(user, ".", "_")
	}
	return user, nil
}

func (c gkeClusterContext) bindRoles(plan Plan) error {
	user, err := c.username(false)
	if err != nil {
		return err
	}
	cmd := fmt.Sprintf("kubectl create clusterrolebinding cluster-admin-binding --clusterrole=cluster-admin --user=%s", user)
	if plan.Gke.Private {
		log.Printf("this is a private cluster, please bind roles manually from an authorized VM with the following command:\n$ %s\n", cmd)
		return nil
	}
	log.Println("Binding roles...")
	return exec.NewCommand(cmd).Run()
}

func (d *GKEDriver) GetCredentials() error {
	log.Println("Verifying gcloud authentication...")
	// --verbosity flag here disables warnings, and survey output.
	out, err := exec.NewCommand(`gcloud auth list --filter=status:ACTIVE --format="value(account)" --verbosity error`).StdoutOnly().OutputList()
	if err != nil {
		return fmt.Errorf("while retrieving list of credentialed gcloud accounts: %w", err)
	}
	if d.plan.Gke.GCloudProject == "" {
		return fmt.Errorf("while retrieving google cloud project: missing key %s", GoogleCloudProjectCtxKey)
	}
	// If there's no authenticated user, or the authenticated user doesn't exist in the configured project
	// then we need to authenticate with what's within vault.
	if len(out) == 0 || (len(out) > 0 && !strings.Contains(out[0], d.plan.Gke.GCloudProject)) {
		if err := authToGCP(
			d.vaultClient, GKEVaultPath, GKEServiceAccountVaultFieldName,
			d.plan.ServiceAccount, false, d.plan.Gke.GCloudProject,
		); err != nil {
			return fmt.Errorf("while authenticating to GCP: %w", err)
		}
	}
	log.Println("Getting credentials...")
	return getCredentialsFromClusters(d.clusters)
}

func getCredentialsFromClusters(clusters []clusterContext) error {
	var errs error
	for _, cCtx := range clusters {
		if err := cCtx.getCredentials(); err != nil {
			errs = errors.Join(errs, fmt.Errorf("region %s: %w", cCtx.region(), err))
			continue
		}
		return nil
	}
	if errs == nil {
		return errors.New("no regions configured")
	}
	return fmt.Errorf("failed to get credentials from configured regions: %w", errs)
}

func (c gkeClusterContext) getCredentials() error {
	cmd := "gcloud container clusters --project {{.GCloudProject}} get-credentials {{.ClusterName}} --region {{.Region}}"
	return exec.NewCommand(cmd).AsTemplate(c).Run()
}

// waitForClusterOperations waits for any running GKE cluster operations to complete.
// GKE does not allow cluster deletion while operations like auto-upgrades, auto-repairs,
// or node pool modifications are in progress. Attempting to delete during such operations
// results in: "Cluster is running incompatible operation" error.
func (c gkeClusterContext) waitForClusterOperations() error {
	log.Println("Checking for running cluster operations...")
	cmd := `gcloud container operations list --project {{.GCloudProject}} --region {{.Region}} --filter="targetLink~{{.ClusterName}} AND status=RUNNING" --format="value(name)"`
	operations, err := exec.NewCommand(cmd).AsTemplate(c).WithoutStreaming().OutputList()
	if err != nil {
		return fmt.Errorf("while listing cluster operations: %w", err)
	}

	for _, op := range operations {
		log.Printf("Waiting for operation %s to complete...", op)
		waitCmd := fmt.Sprintf(`gcloud container operations wait %s --project {{.GCloudProject}} --region {{.Region}}`, op)
		if err := exec.NewCommand(waitCmd).AsTemplate(c).Run(); err != nil {
			return fmt.Errorf("while waiting for operation %s: %w", op, err)
		}
	}

	return nil
}

func (c gkeClusterContext) delete() error {
	// Retry deletion up to 3 times to handle transient failures. GKE cluster deletion can fail
	// due to race conditions where new operations start between our check and delete attempt,
	// or due to temporary API errors.
	const maxDeleteAttempts = 3
	var lastErr error

	for attempt := 1; attempt <= maxDeleteAttempts; attempt++ {
		if attempt > 1 {
			log.Printf("Retrying cluster deletion (attempt %d/%d)...", attempt, maxDeleteAttempts)
		}
		if exists, err := c.exists(); err != nil {
			lastErr = err
			log.Printf("Error checking for cluster existence: %v", err)
			continue
		} else if !exists {
			return nil
		}

		if err := c.waitForClusterOperations(); err != nil {
			lastErr = err
			log.Printf("Error waiting for cluster operations: %v", err)
			continue
		}

		log.Println("Deleting cluster...")
		cmd := "gcloud --quiet --project {{.GCloudProject}} container clusters delete {{.ClusterName}} --region {{.Region}}"
		if err := exec.NewCommand(cmd).AsTemplate(c).Run(); err != nil {
			lastErr = err
			log.Printf("Error deleting cluster: %v", err)
			continue
		}

		// Deletion succeeded, break out of retry loop
		lastErr = nil
		break
	}

	if lastErr != nil {
		return fmt.Errorf("failed to delete cluster after %d attempts: %w", maxDeleteAttempts, lastErr)
	}

	// Deleting clusters in GKE does not delete associated disks, we have to delete them manually.
	diskCmd := `gcloud compute disks list --filter='labels.cluster_name={{.ClusterName}} AND labels.region={{.Region}} AND -users:*' --format="value[separator=','](name,zone)" --project {{.GCloudProject}}`
	disks, err := exec.NewCommand(diskCmd).AsTemplate(c).StdoutOnly().OutputList()
	if err != nil {
		return err
	}
	if err := c.deleteDisks(disks); err != nil {
		return err
	}
	deletedDisks := len(disks)

	// This is the "legacy" way to detect orphaned disks. Keep using it while all disks do not have labels.
	diskCmd = `gcloud compute disks list --filter="name~^gke-{{.PVCPrefix}}.*-pvc-.+" --format="value[separator=','](name,zone)" --project {{.GCloudProject}}`
	disks, err = exec.NewCommand(diskCmd).AsTemplate(c).StdoutOnly().OutputList()
	if err != nil {
		return err
	}
	if err := c.deleteDisks(disks); err != nil {
		return err
	}
	deletedDisks += len(disks)
	if deletedDisks == 0 {
		log.Println("No GCE persistent disks deleted")
	} else {
		log.Printf("%d GCE persistent disks deleted", deletedDisks)
	}

	return nil
}

func (c gkeClusterContext) deleteDisks(disks []string) error {
	for _, disk := range disks {
		nameZone := strings.Split(disk, ",")
		if len(nameZone) != 2 {
			return fmt.Errorf("disk name and zone contained unexpected number of fields")
		}

		name, zone := nameZone[0], nameZone[1]
		cmd := `gcloud compute disks delete {{.Name}} --project {{.GCloudProject}} --zone {{.Zone}} --quiet`
		err := exec.NewCommand(cmd).
			AsTemplate(map[string]any{
				GoogleCloudProjectCtxKey: c[GoogleCloudProjectCtxKey],
				"Name":                   name,
				"Zone":                   zone,
			}).
			Run()
		if err != nil {
			return err
		}
	}
	return nil
}

func (d *GKEDriver) newBucketManager(plan Plan, c clusterContext) (bucket.Manager, error) {
	// Use VaultManager for pre-provisioned buckets
	if plan.Bucket.FromVault {
		return newVaultBucketManager(GKEDriverID, d.vaultClient)
	}

	// Use GCSManager for dynamic bucket creation
	if err := bucket.ValidateShellArg(plan.Gke.GCloudProject, "GCP project"); err != nil {
		return nil, err
	}
	if plan.Bucket.StorageClass != "" {
		if err := bucket.ValidateShellArg(plan.Bucket.StorageClass, "storage class"); err != nil {
			return nil, err
		}
	}
	cfg, err := newBucketConfig(plan, c.values(), c.region())
	if err != nil {
		return nil, err
	}
	return bucket.NewGCSManager(cfg, plan.Gke.GCloudProject, plan.Bucket.StorageClass), nil
}

func (d *GKEDriver) bucketManagerFactory(plan Plan, c clusterContext) func() (bucket.Manager, error) {
	return func() (bucket.Manager, error) {
		if d.newBucketManagerFn != nil {
			return d.newBucketManagerFn(plan, c)
		}
		return d.newBucketManager(plan, c)
	}
}

func (d *GKEDriver) Cleanup(prefix string, olderThan time.Duration) error {
	if d.plan.Gke.GCloudProject == "" {
		gCloudProject, err := vault.Get(d.vaultClient, GKEVaultPath, GKEProjectVaultFieldName)
		if err != nil {
			return err
		}
		d.plan.Gke.GCloudProject = gCloudProject
	}

	if err := authToGCP(
		d.vaultClient, GKEVaultPath, GKEServiceAccountVaultFieldName,
		d.plan.ServiceAccount, false, d.plan.Gke.GCloudProject,
	); err != nil {
		return err
	}
	return d.cleanup(prefix, olderThan)
}

func (d *GKEDriver) cleanup(prefix string, olderThan time.Duration) error {
	var errs error
	sinceDate := time.Now().Add(-olderThan)
	for _, cCtx := range d.clusters {
		cleanupCtx := cCtx.withProject(d.plan.Gke.GCloudProject)
		clusters, err := cleanupCtx.listClusters(prefix, sinceDate)
		if err != nil {
			errs = errors.Join(errs, fmt.Errorf("list clusters in region %s: %w", cleanupCtx.region(), err))
			continue
		}

		for _, cluster := range clusters {
			clusterCtx := cleanupCtx.withClusterName(cluster)
			clusterPlan := d.plan
			clusterPlan.ClusterName = cluster
			if err := deleteBucketIfConfigured(clusterPlan, d.bucketManagerFactory(clusterPlan, clusterCtx)); err != nil {
				log.Printf("warning: bucket deletion failed for cluster %s, will continue: %v", cluster, err)
				errs = errors.Join(errs, fmt.Errorf("delete bucket for cluster %s in region %s: %w", cluster, clusterCtx.region(), err))
			}
			if err = clusterCtx.delete(); err != nil {
				log.Printf("while deleting cluster %s: %v", cluster, err.Error())
				errs = errors.Join(errs, fmt.Errorf("delete cluster %s in region %s: %w", cluster, clusterCtx.region(), err))
				continue
			}
		}
	}

	return errs
}
