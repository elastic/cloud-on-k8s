// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package runner

import (
	"fmt"
	"log"
	"strings"
)

const (
	GkeDriverID                     = "gke"
	GkeVaultPath                    = "secret/devops-ci/cloud-on-k8s/ci-gcp-k8s-operator"
	GkeServiceAccountVaultFieldName = "service-account"
	GkeConfigFileName               = "deployer-config-gke.yml"
	DefaultGkeRunConfigTemplate     = `id: gke-dev
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
	drivers[GkeDriverID] = &GkeDriverFactory{}
}

type GkeDriverFactory struct {
}

type GkeDriver struct {
	plan Plan
	ctx  map[string]interface{}
}

func (gdf *GkeDriverFactory) Create(plan Plan) (Driver, error) {
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

	return &GkeDriver{
		plan: plan,
		ctx: map[string]interface{}{
			"GCloudProject":     plan.Gke.GCloudProject,
			"ClusterName":       plan.ClusterName,
			"PVCPrefix":         pvcPrefix,
			"Region":            plan.Gke.Region,
			"AdminUsername":     plan.Gke.AdminUsername,
			"KubernetesVersion": plan.KubernetesVersion,
			"MachineType":       plan.MachineType,
			"LocalSsdCount":     plan.Gke.LocalSsdCount,
			"GcpScopes":         plan.Gke.GcpScopes,
			"NodeCountPerZone":  plan.Gke.NodeCountPerZone,
			"ClusterIPv4CIDR":   clusterIPv4CIDR,
			"ServicesIPv4CIDR":  servicesIPv4CIDR,
		},
	}, nil
}

func (d *GkeDriver) Execute() error {
	if err := authToGCP(
		d.plan.VaultInfo, GkeVaultPath, GkeServiceAccountVaultFieldName,
		d.plan.ServiceAccount, false, d.ctx["GCloudProject"],
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

		if err := setupDisks(d.plan); err != nil {
			return err
		}
		if err := createStorageClass(); err != nil {
			return err
		}
	default:
		err = fmt.Errorf("unknown operation %s", d.plan.Operation)
	}

	return err
}

func (d *GkeDriver) clusterExists() (bool, error) {
	log.Println("Checking if cluster exists...")

	cmd := "gcloud beta container clusters --project {{.GCloudProject}} describe {{.ClusterName}} --region {{.Region}}"
	contains, err := NewCommand(cmd).AsTemplate(d.ctx).WithoutStreaming().OutputContainsAny("Not found")
	if contains {
		return false, nil
	}

	return err == nil, err
}

func (d *GkeDriver) create() error {
	log.Println("Creating cluster...")

	opts := []string{}
	if d.plan.Psp {
		opts = append(opts, "--enable-pod-security-policy")
	}

	if d.plan.Gke.NetworkPolicy {
		opts = append(opts, "--enable-network-policy")
	}

	if d.plan.Gke.Private {
		opts = append(opts, "--create-subnetwork name={{.ClusterName}}-private-subnet", "--enable-master-authorized-networks", "--enable-ip-alias", "--enable-private-nodes", "--enable-private-endpoint", "--master-ipv4-cidr", "172.16.0.32/28")
	} else {
		opts = append(opts, "--create-subnetwork range={{.ClusterIPv4CIDR}}", "--cluster-ipv4-cidr={{.ClusterIPv4CIDR}}", "--services-ipv4-cidr={{.ServicesIPv4CIDR}}")
	}

	return NewCommand(`gcloud beta container --project {{.GCloudProject}} clusters create {{.ClusterName}} ` +
		`--region {{.Region}} --username {{.AdminUsername}} --cluster-version {{.KubernetesVersion}} ` +
		`--machine-type {{.MachineType}} --image-type COS --disk-type pd-ssd --disk-size 30 ` +
		`--local-ssd-count {{.LocalSsdCount}} --scopes {{.GcpScopes}} --num-nodes {{.NodeCountPerZone}} ` +
		`--enable-stackdriver-kubernetes --addons HorizontalPodAutoscaling,HttpLoadBalancing ` +
		`--no-enable-autoupgrade --no-enable-autorepair --enable-ip-alias --metadata disable-legacy-endpoints=true ` +
		`--network projects/{{.GCloudProject}}/global/networks/default ` +
		strings.Join(opts, " ")).
		AsTemplate(d.ctx).
		Run()
}

func (d *GkeDriver) bindRoles() error {
	user, err := NewCommand(`gcloud auth list --filter=status:ACTIVE --format="value(account)"`).Output()
	if err != nil {
		return err
	}
	cmd := "kubectl create clusterrolebinding cluster-admin-binding --clusterrole=cluster-admin --user=" + user
	if d.plan.Gke.Private {
		log.Printf("this is a private cluster, please bind roles manually from an authorized VM with the following command:\n$ %s\n", cmd)
		return nil
	}
	log.Println("Binding roles...")
	return NewCommand(cmd).Run()
}

func (d *GkeDriver) GetCredentials() error {
	log.Println("Getting credentials...")
	cmd := "gcloud container clusters --project {{.GCloudProject}} get-credentials {{.ClusterName}} --region {{.Region}}"
	return NewCommand(cmd).AsTemplate(d.ctx).Run()
}

func (d *GkeDriver) delete() error {
	log.Println("Deleting cluster...")
	cmd := "gcloud beta --quiet --project {{.GCloudProject}} container clusters delete {{.ClusterName}} --region {{.Region}}"
	if err := NewCommand(cmd).AsTemplate(d.ctx).Run(); err != nil {
		return err
	}

	// Deleting clusters in GKE does not delete associated disks, we have to delete them manually.
	cmd = `gcloud compute disks list --filter="name~^gke-{{.PVCPrefix}}.*-pvc-.+" --format="value[separator=','](name,zone)" --project {{.GCloudProject}}`
	disks, err := NewCommand(cmd).AsTemplate(d.ctx).StdoutOnly().OutputList()
	if err != nil {
		return err
	}

	for _, disk := range disks {
		nameZone := strings.Split(disk, ",")
		if len(nameZone) != 2 {
			return fmt.Errorf("disk name and zone contained unexpected number of fields")
		}

		name, zone := nameZone[0], nameZone[1]
		cmd = `gcloud compute disks delete {{.Name}} --project {{.GCloudProject}} --zone {{.Zone}} --quiet`
		err := NewCommand(cmd).
			AsTemplate(map[string]interface{}{
				"GCloudProject": d.plan.Gke.GCloudProject,
				"Name":          name,
				"Zone":          zone,
			}).
			Run()
		if err != nil {
			return err
		}
	}

	return nil
}
