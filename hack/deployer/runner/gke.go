// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package runner

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/ghodss/yaml"
	storagev1 "k8s.io/api/storage/v1"

	"github.com/elastic/cloud-on-k8s/v2/hack/deployer/exec"
	"github.com/elastic/cloud-on-k8s/v2/hack/deployer/runner/kyverno"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/vault"
)

const (
	storageClassPrefix              = "e2e-"
	GKEDriverID                     = "gke"
	GKEVaultPath                    = "ci-gcp-k8s-operator"
	GKEServiceAccountVaultFieldName = "service-account"
	GKEProjectVaultFieldName        = "gcloud-project"
	GoogleCloudProjectCtxKey        = "GCloudProject"
	DefaultGKERunConfigTemplate     = `id: gke-dev
overrides:
  clusterName: %s-dev-cluster
  gke:
    gCloudProject: %s
`
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
	ctx         map[string]interface{}
	vaultClient vault.Client
}

func (gdf *GKEDriverFactory) Create(plan Plan) (Driver, error) {
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

	return &GKEDriver{
		plan: plan,
		ctx: map[string]interface{}{
			GoogleCloudProjectCtxKey: plan.Gke.GCloudProject,
			"ClusterName":            plan.ClusterName,
			"PVCPrefix":              pvcPrefix,
			"PlanId":                 plan.Id,
			"Region":                 plan.Gke.Region,
			"KubernetesVersion":      plan.KubernetesVersion,
			"MachineType":            plan.MachineType,
			"LocalSsdCount":          plan.Gke.LocalSsdCount,
			"GcpScopes":              plan.Gke.GcpScopes,
			"NodeCountPerZone":       plan.Gke.NodeCountPerZone,
			"ClusterIPv4CIDR":        clusterIPv4CIDR,
			"ServicesIPv4CIDR":       servicesIPv4CIDR,
		},
		vaultClient: c,
	}, nil
}

func (d *GKEDriver) Execute() error {
	if err := authToGCP(
		d.vaultClient, GKEVaultPath, GKEServiceAccountVaultFieldName,
		d.plan.ServiceAccount, false, d.ctx[GoogleCloudProjectCtxKey],
	); err != nil {
		return err
	}

	exists, err := d.clusterExists()
	if err != nil {
		return err
	}

	switch d.plan.Operation {
	case DeleteAction:
		if exists {
			err = d.delete()
		} else {
			log.Printf("not deleting as cluster doesn't exist")
		}
	case CreateAction:
		if exists {
			log.Printf("not creating as cluster exists")
		} else {
			if err := d.create(); err != nil {
				return err
			}

			if err := d.bindRoles(); err != nil {
				return err
			}
		}

		if d.plan.Gke.Private {
			log.Printf("a private cluster has been created, please retrieve credentials manually and create storage class and provider if needed")
			log.Printf("to authorize a VM to access this cluster run the following command:\n"+
				"$ gcloud container clusters update %s"+
				" --region %s "+
				"--enable-master-authorized-networks"+
				" --master-authorized-networks  <VM IP>/32",
				d.plan.ClusterName, d.plan.Gke.Region)
			log.Printf("you can then retrieve the credentials with the following command:\n"+
				"$ gcloud container clusters get-credentials %s"+
				" --region %s "+
				" --project %s",
				d.plan.ClusterName, d.plan.Gke.Region, d.plan.Gke.GCloudProject)
			return nil
		}

		if err := d.GetCredentials(); err != nil {
			return err
		}

		if err := d.copyBuiltInStorageClasses(); err != nil {
			return err
		}

		if err := setupDisks(d.plan); err != nil {
			return err
		}
		if err := createStorageClass(); err != nil {
			return err
		}
		if d.plan.EnforceSecurityPolicies {
			if err := kyverno.Install(); err != nil {
				return err
			}
			// apply extra policies to prevent use of unlabeled storage classes which might escape garbage collection in CI
			if err := apply(kyverno.GKEPolicies); err != nil {
				return err
			}
		}
	default:
		err = fmt.Errorf("unknown operation %s", d.plan.Operation)
	}

	return err
}

const (
	GoogleComputeEngineStorageProvider = "pd.csi.storage.gke.io"
)

// copyBuiltInStorageClasses adds the "labels" parameter to copies of the built-in  GCE storage classes.
// These labels are automatically applied to GCE Persistent Disks provisioned using these storage classes.
func (d *GKEDriver) copyBuiltInStorageClasses() error {
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

	labels, err := d.resourcesLabels()
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

func (d *GKEDriver) resourcesLabels() (string, error) {
	username, err := d.username(true)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(
		"username=%s,cluster_name=%s,plan_id=%s,region=%s",
		username, d.ctx["ClusterName"], d.ctx["PlanId"], d.ctx["Region"],
	), nil
}

func (d *GKEDriver) clusterExists() (bool, error) {
	log.Println("Checking if cluster exists...")

	cmd := "gcloud beta container clusters --project {{.GCloudProject}} describe {{.ClusterName}} --region {{.Region}}"
	contains, err := exec.NewCommand(cmd).AsTemplate(d.ctx).WithoutStreaming().OutputContainsAny("Not found")
	if contains {
		return false, nil
	}

	return err == nil, err
}

func (d *GKEDriver) create() error {
	log.Println("Creating cluster...")

	opts := []string{}

	if d.plan.Gke.NetworkPolicy {
		if d.plan.Gke.Autopilot {
			return fmt.Errorf("--enable-network-policy must not be set if autopilot is enabled")
		}
		opts = append(opts, "--enable-network-policy")
	}

	if d.plan.Gke.Private {
		opts = append(opts, "--create-subnetwork name={{.ClusterName}}-private-subnet", "--enable-master-authorized-networks", "--enable-ip-alias", "--enable-private-nodes", "--enable-private-endpoint", "--master-ipv4-cidr", "172.16.0.32/28")
	} else {
		opts = append(opts, "--create-subnetwork range={{.ClusterIPv4CIDR}}", "--cluster-ipv4-cidr={{.ClusterIPv4CIDR}}", "--services-ipv4-cidr={{.ServicesIPv4CIDR}}")
	}

	labels, err := d.resourcesLabels()
	if err != nil {
		return err
	}
	labels = fmt.Sprintf("%s,%s", strings.Join(toList(elasticTags), ","), labels)

	var createGKEClusterCommand string
	if !d.plan.Gke.Autopilot {
		createGKEClusterCommand = `gcloud beta container --quiet --project {{.GCloudProject}} clusters create {{.ClusterName}} ` +
			`--labels "` + labels + `" --region {{.Region}} --no-enable-basic-auth --cluster-version {{.KubernetesVersion}} ` +
			`--machine-type {{.MachineType}} --disk-type pd-ssd --disk-size 40 ` +
			`--local-ssd-count {{.LocalSsdCount}} --scopes {{.GcpScopes}} --num-nodes {{.NodeCountPerZone}} ` +
			`--addons HorizontalPodAutoscaling,HttpLoadBalancing ` +
			`--no-enable-autoupgrade --no-enable-autorepair --enable-ip-alias --metadata disable-legacy-endpoints=true ` +
			`--network projects/{{.GCloudProject}}/global/networks/default ` +
			strings.Join(opts, " ")
	} else {
		// Autopilot cluster.
		log.Println("autopilot cluster enabled")
		createGKEClusterCommand = `gcloud beta container --quiet --project {{.GCloudProject}} clusters create-auto {{.ClusterName}} ` +
			`--region {{.Region}} --cluster-version {{.KubernetesVersion}} ` +
			`--scopes {{.GcpScopes}} --network projects/{{.GCloudProject}}/global/networks/default ` +
			strings.Join(opts, " ")
	}

	err = exec.NewCommand(createGKEClusterCommand).
		AsTemplate(d.ctx).
		Run()

	if err != nil {
		return err
	}

	// Since gcloud doesn't support labels at creation time for autopilot clusters, update the labels after creation.
	if d.plan.Gke.Autopilot {
		return exec.NewCommand(`gcloud beta container --quiet --project {{.GCloudProject}} clusters update {{.ClusterName}} --region {{.Region}} --update-labels="` + labels + `"`).
			AsTemplate(d.ctx).
			Run()
	}
	return nil
}

// username attempts to extract the username from the current account.
// When used in labels the "unqualified" parameter should be set to true, it's because only lowercase letters ([a-z]),
// numeric characters ([0-9]), underscores (_) and dashes (-) are allowed as label values.
func (d *GKEDriver) username(unqualified bool) (string, error) {
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

func (d *GKEDriver) bindRoles() error {
	user, err := d.username(false)
	if err != nil {
		return err
	}
	cmd := fmt.Sprintf("kubectl create clusterrolebinding cluster-admin-binding --clusterrole=cluster-admin --user=%s", user)
	if d.plan.Gke.Private {
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
	gcloudProjectInt, ok := d.ctx[GoogleCloudProjectCtxKey]
	if !ok {
		return fmt.Errorf("while retrieving google cloud project: missing key %s", GoogleCloudProjectCtxKey)
	}
	gCloudProject, ok := gcloudProjectInt.(string)
	if !ok {
		return fmt.Errorf("while retrieving google cloud project: key %s was not a string, was %T ", GoogleCloudProjectCtxKey, gcloudProjectInt)
	}
	// If there's no authenticated user, or the authenticated user doesn't exist in the configured project
	// then we need to authenticate with what's within vault.
	if len(out) == 0 || (len(out) > 0 && !strings.Contains(out[0], gCloudProject)) {
		if err := authToGCP(
			d.vaultClient, GKEVaultPath, GKEServiceAccountVaultFieldName,
			d.plan.ServiceAccount, false, d.ctx[GoogleCloudProjectCtxKey],
		); err != nil {
			return fmt.Errorf("while authenticating to GCP: %w", err)
		}
	}
	log.Println("Getting credentials...")
	cmd := "gcloud container clusters --project {{.GCloudProject}} get-credentials {{.ClusterName}} --region {{.Region}}"
	return exec.NewCommand(cmd).AsTemplate(d.ctx).Run()
}

func (d *GKEDriver) delete() error {
	log.Println("Deleting cluster...")
	cmd := "gcloud beta --quiet --project {{.GCloudProject}} container clusters delete {{.ClusterName}} --region {{.Region}}"
	if err := exec.NewCommand(cmd).AsTemplate(d.ctx).Run(); err != nil {
		return err
	}

	// Deleting clusters in GKE does not delete associated disks, we have to delete them manually.
	cmd = `gcloud compute disks list --filter='labels.cluster_name={{.ClusterName}} AND labels.region={{.Region}} AND -users:*' --format="value[separator=','](name,zone)" --project {{.GCloudProject}}`
	disks, err := exec.NewCommand(cmd).AsTemplate(d.ctx).StdoutOnly().OutputList()
	if err != nil {
		return err
	}
	if err := d.deleteDisks(disks); err != nil {
		return err
	}
	deletedDisks := len(disks)

	// This is the "legacy" way to detect orphaned disks. Keep using it while all disks do not have labels.
	cmd = `gcloud compute disks list --filter="name~^gke-{{.PVCPrefix}}.*-pvc-.+" --format="value[separator=','](name,zone)" --project {{.GCloudProject}}`
	disks, err = exec.NewCommand(cmd).AsTemplate(d.ctx).StdoutOnly().OutputList()
	if err != nil {
		return err
	}
	if err := d.deleteDisks(disks); err != nil {
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

func (d *GKEDriver) deleteDisks(disks []string) error {
	for _, disk := range disks {
		nameZone := strings.Split(disk, ",")
		if len(nameZone) != 2 {
			return fmt.Errorf("disk name and zone contained unexpected number of fields")
		}

		name, zone := nameZone[0], nameZone[1]
		cmd := `gcloud compute disks delete {{.Name}} --project {{.GCloudProject}} --zone {{.Zone}} --quiet`
		err := exec.NewCommand(cmd).
			AsTemplate(map[string]interface{}{
				GoogleCloudProjectCtxKey: d.plan.Gke.GCloudProject,
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

func (d *GKEDriver) Cleanup(prefix string, olderThan time.Duration) error {
	if d.ctx[GoogleCloudProjectCtxKey] == "" {
		gCloudProject, err := vault.Get(d.vaultClient, GKEVaultPath, GKEProjectVaultFieldName)
		if err != nil {
			return err
		}
		d.ctx[GoogleCloudProjectCtxKey] = gCloudProject
	}

	if err := authToGCP(
		d.vaultClient, GKEVaultPath, GKEServiceAccountVaultFieldName,
		d.plan.ServiceAccount, false, d.ctx[GoogleCloudProjectCtxKey],
	); err != nil {
		return err
	}

	sinceDate := time.Now().Add(-olderThan)
	d.ctx["Date"] = sinceDate.Format(time.RFC3339)
	d.ctx["E2EClusterNamePrefix"] = prefix

	cmd := `gcloud container clusters list --verbosity error --region={{.Region}} --format="value(name)" --filter="createTime<{{.Date}} AND name~{{.E2EClusterNamePrefix}}.*"`
	clusters, err := exec.NewCommand(cmd).AsTemplate(d.ctx).OutputList()
	if err != nil {
		return err
	}

	for _, cluster := range clusters {
		d.ctx["ClusterName"] = cluster
		if err = d.delete(); err != nil {
			log.Printf("while deleting cluster %s: %v", cluster, err.Error())
			continue
		}
	}
	return nil
}
