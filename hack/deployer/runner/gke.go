// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package runner

import (
	"fmt"
	"log"
	"os"
	"strings"
)

const (
	GKEDriverID                     = "gke"
	GKEVaultPath                    = "secret/devops-ci/cloud-on-k8s/ci-gcp-k8s-operator"
	GKEServiceAccountVaultFieldName = "service-account"
	GKEConfigFileName               = "deployer-config-gke.yml"
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
	GKEStorageProvisioner   = "kubernetes.io/no-provisioner"
	defaultClusterIPv4CIDR  = "/20"
	defaultServicesIPv4CIDR = "/20"
)

func init() {
	drivers[GKEDriverID] = &GKEDriverFactory{}
}

type GKEDriverFactory struct {
}

type GKEDriver struct {
	plan Plan
	ctx  map[string]interface{}
}

func (gdf *GKEDriverFactory) Create(plan Plan) (Driver, error) {
	pvcPrefix := plan.ClusterName
	if len(pvcPrefix) > pvcPrefixMaxLength {
		pvcPrefix = pvcPrefix[0:pvcPrefixMaxLength]
	}

	clusterIPv4CIDR := defaultClusterIPv4CIDR
	if plan.GKE.ClusterIPv4CIDR != "" {
		clusterIPv4CIDR = plan.GKE.ClusterIPv4CIDR
	}

	servicesIPv4CIDR := defaultServicesIPv4CIDR
	if plan.GKE.ServicesIPv4CIDR != "" {
		servicesIPv4CIDR = plan.GKE.ServicesIPv4CIDR
	}

	return &GKEDriver{
		plan: plan,
		ctx: map[string]interface{}{
			"GCloudProject":     plan.GKE.GCloudProject,
			"ClusterName":       plan.ClusterName,
			"PVCPrefix":         pvcPrefix,
			"Region":            plan.GKE.Region,
			"AdminUsername":     plan.GKE.AdminUsername,
			"KubernetesVersion": plan.KubernetesVersion,
			"MachineType":       plan.MachineType,
			"LocalSsdCount":     plan.GKE.LocalSsdCount,
			"GcpScopes":         plan.GKE.GcpScopes,
			"NodeCountPerZone":  plan.GKE.NodeCountPerZone,
			"ClusterIPv4CIDR":   clusterIPv4CIDR,
			"ServicesIPv4CIDR":  servicesIPv4CIDR,
		},
	}, nil
}

func (d *GKEDriver) Execute() error {
	if err := d.auth(); err != nil {
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

		if err := d.GetCredentials(); err != nil {
			return err
		}

		if err := d.configureDocker(); err != nil {
			return err
		}
		if err := createStorageClass(GKEStorageProvisioner); err != nil {
			return err
		}
		if err := d.createSsdProvider(); err != nil {
			return err
		}
	default:
		err = fmt.Errorf("unknown operation %s", d.plan.Operation)
	}

	return err
}

func (d *GKEDriver) createSsdProvider() error {
	return NewCommand(fmt.Sprintf(`cat <<EOF | kubectl apply -f -
%s
EOF`, GKESSDProvisioner)).Run()
}

func (d *GKEDriver) auth() error {
	if d.plan.ServiceAccount {
		log.Println("Authenticating as service account...")

		client, err := NewClient(*d.plan.VaultInfo)
		if err != nil {
			return err
		}

		keyFileName := "gke_service_account_key.json"
		defer os.Remove(keyFileName)
		if err := client.ReadIntoFile(keyFileName, GKEVaultPath, GKEServiceAccountVaultFieldName); err != nil {
			return err
		}

		return NewCommand("gcloud auth activate-service-account --key-file=" + keyFileName).Run()
	}

	log.Println("Authenticating as user...")
	accounts, err := NewCommand(`gcloud auth list "--format=value(account)"`).StdoutOnly().WithoutStreaming().Output()
	if err != nil {
		return err
	}

	if len(accounts) > 0 {
		return nil
	}

	return NewCommand("gcloud auth login").Run()
}

func (d *GKEDriver) clusterExists() (bool, error) {
	log.Println("Checking if cluster exists...")

	cmd := "gcloud beta container clusters --project {{.GCloudProject}} describe {{.ClusterName}} --region {{.Region}}"
	contains, err := NewCommand(cmd).AsTemplate(d.ctx).WithoutStreaming().OutputContainsAny("Not found")
	if contains {
		return false, nil
	}

	return err == nil, err
}

func (d *GKEDriver) create() error {
	log.Println("Creating cluster...")

	opts := []string{}
	if d.plan.Psp {
		opts = append(opts, "--enable-pod-security-policy")
	}

	return NewCommand(`gcloud beta container --project {{.GCloudProject}} clusters create {{.ClusterName}} ` +
		`--region {{.Region}} --username {{.AdminUsername}} --cluster-version {{.KubernetesVersion}} ` +
		`--machine-type {{.MachineType}} --image-type COS --disk-type pd-ssd --disk-size 30 ` +
		`--local-ssd-count {{.LocalSsdCount}} --scopes {{.GcpScopes}} --num-nodes {{.NodeCountPerZone}} ` +
		`--enable-stackdriver-kubernetes --addons HorizontalPodAutoscaling,HttpLoadBalancing ` +
		`--no-enable-autoupgrade --no-enable-autorepair --enable-ip-alias --metadata disable-legacy-endpoints=true ` +
		`--network projects/{{.GCloudProject}}/global/networks/default ` +
		`--create-subnetwork range={{.ClusterIPv4CIDR}} --cluster-ipv4-cidr={{.ClusterIPv4CIDR}} --services-ipv4-cidr={{.ServicesIPv4CIDR}} ` +
		strings.Join(opts, " ")).
		AsTemplate(d.ctx).
		Run()
}

func (d *GKEDriver) bindRoles() error {
	log.Println("Binding roles...")
	user, err := NewCommand(`gcloud auth list --filter=status:ACTIVE --format="value(account)"`).Output()
	if err != nil {
		return err
	}
	cmd := "kubectl create clusterrolebinding cluster-admin-binding --clusterrole=cluster-admin --user=" + user
	return NewCommand(cmd).Run()
}

func (d *GKEDriver) configureDocker() error {
	log.Println("Configuring Docker...")
	return NewCommand("gcloud auth configure-docker --quiet").Run()
}

func (d *GKEDriver) GetCredentials() error {
	log.Println("Getting credentials...")
	cmd := "gcloud container clusters --project {{.GCloudProject}} get-credentials {{.ClusterName}} --region {{.Region}}"
	return NewCommand(cmd).AsTemplate(d.ctx).Run()
}

func (d *GKEDriver) delete() error {
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
				"GCloudProject": d.plan.GKE.GCloudProject,
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
