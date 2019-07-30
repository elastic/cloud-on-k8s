// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"
)

const (
	GkeDriverId                   = "gke"
	GkeServiceAccountKeyVaultName = "secret/cloud-team/cloud-ci/ci-gcp-k8s-operator"
)

func init() {
	drivers[GkeDriverId] = &GkeDriverFactory{}
}

type GkeDriverFactory struct {
}

type GkeDriver struct {
	plan Plan
}

func (gdf *GkeDriverFactory) Create(plan Plan) (Driver, error) {
	return &GkeDriver{plan: plan}, nil
}

func (d *GkeDriver) Execute() error {
	if err := d.auth(); err != nil {
		return err
	}

	exists, err := d.clusterExists()
	if err != nil {
		return err
	}

	switch d.plan.Operation {
	case "delete":
		if exists {
			err = d.delete()
		} else {
			log.Printf("not deleting as cluster doesn't exist")
		}
	case "create":
		if exists {
			log.Printf("not creating as cluster exists")
		} else {
			if err := d.configSsh(); err != nil {
				return err
			}

			if err := d.create(); err != nil {
				return err
			}

			if err := d.bindRoles(); err != nil {
				return err
			}
		}

		if err := d.getCredentials(); err != nil {
			return err
		}

		if d.plan.VmMapMax {
			if err := d.setMaxMapCount(); err != nil {
				return err
			}
		}

		if err := d.configureDocker(); err != nil {
			return err
		}
	default:
		err = fmt.Errorf("unknown operation %s", d.plan.Operation)
	}

	return err
}

func (d *GkeDriver) auth() error {
	if d.plan.ServiceAccount {
		return d.serviceAuth()
	} else {
		return d.userAuth()
	}
}

func (d *GkeDriver) serviceAuth() error {
	log.Println("Authenticating as service account...")

	serviceAccountKey, err := ReadVault(d.plan.VaultAddress, d.plan.VaultRoleId, d.plan.VaultSecretId, GkeServiceAccountKeyVaultName)
	if err != nil {
		return err
	}

	file, err := ioutil.TempFile(".", "gke_service_account_key-")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())

	if err := ioutil.WriteFile(file.Name(), []byte(serviceAccountKey), 0644); err != nil {
		return err
	}

	return NewCommand("gcloud auth activate-service-account --key-file={{.FileName}}").
		AsTemplate(map[string]interface{}{
			"FileName": file.Name(),
		}).
		Run()
}

func (d *GkeDriver) userAuth() error {
	log.Println("Authenticating as user...")
	return NewCommand("gcloud auth login").Run()
}

func (d *GkeDriver) clusterExists() (bool, error) {
	log.Println("Checking if cluster exists...")
	out, err := NewCommand("gcloud beta container clusters --project {{.GCloudProject}} describe {{.ClusterName}} --region {{.Region}}").
		AsTemplate(map[string]interface{}{
			"GCloudProject": d.plan.Gke.GCloudProject,
			"ClusterName":   d.plan.ClusterName,
			"Region":        d.plan.Gke.Region,
		}).
		WithoutStreaming().
		Output()

	if strings.Contains(out, "Not found") {
		return false, nil
	}

	return err == nil, err
}

func (d *GkeDriver) configSsh() error {
	log.Println("Configuring ssh...")
	return NewCommand("gcloud --quiet --project {{.GCloudProject}} compute config-ssh").
		AsTemplate(map[string]interface{}{
			"GCloudProject": d.plan.Gke.GCloudProject,
		}).
		Run()
}

func (d *GkeDriver) create() error {
	log.Println("Creating cluster...")
	pspOption := ""
	if d.plan.Psp {
		pspOption = "--enable-pod-security-policy"
	}

	return NewCommand(`gcloud beta container --project {{.GCloudProject}} clusters create {{.ClusterName}} ` +
		`--region {{.Region}} --username {{.AdminUsername}} --cluster-version {{.KubernetesVersion}} ` +
		`--machine-type {{.MachineType}} --image-type COS --disk-type pd-ssd --disk-size 30 ` +
		`--local-ssd-count {{.LocalSsdCount}} --scopes {{.GcpScopes}} --num-nodes {{.NodeCountPerZone}} ` +
		`--enable-cloud-logging --enable-cloud-monitoring --addons HorizontalPodAutoscaling,HttpLoadBalancing ` +
		`--no-enable-autoupgrade --no-enable-autorepair --network projects/{{.GCloudProject}}/global/networks/default ` +
		`--subnetwork projects/{{.GCloudProject}}/regions/{{.Region}}/subnetworks/default {{.PspOption}}`).
		AsTemplate(map[string]interface{}{
			"GCloudProject":     d.plan.Gke.GCloudProject,
			"ClusterName":       d.plan.ClusterName,
			"Region":            d.plan.Gke.Region,
			"AdminUsername":     d.plan.Gke.AdminUsername,
			"KubernetesVersion": d.plan.KubernetesVersion,
			"MachineType":       d.plan.MachineType,
			"LocalSsdCount":     d.plan.Gke.LocalSsdCount,
			"GcpScopes":         d.plan.Gke.GcpScopes,
			"NodeCountPerZone":  d.plan.Gke.NodeCountPerZone,
			"PspOption":         pspOption,
		}).
		Run()
}

func (d *GkeDriver) bindRoles() error {
	log.Println("Binding roles...")
	user, err := NewCommand(`gcloud auth list --filter=status:ACTIVE --format="value(account)"`).Output()
	if err != nil {
		return err
	}

	return NewCommand("kubectl create clusterrolebinding cluster-admin-binding --clusterrole=cluster-admin --user={{.User}}").
		AsTemplate(map[string]interface{}{
			"User": user,
		}).
		Run()
}

func (d *GkeDriver) setMaxMapCount() error {
	log.Println("Setting max map count...")
	out, err := NewCommand(`gcloud compute instances list ` +
		`--project={{.GCloudProject}} ` +
		`--filter="metadata.items.key['cluster-name']['value']='{{.ClusterName}}' AND metadata.items.key['cluster-name']['value']!='' " ` +
		`--format="value[separator=','](name,zone)" --verbosity=error`).
		AsTemplate(map[string]interface{}{
			"GCloudProject": d.plan.Gke.GCloudProject,
			"ClusterName":   d.plan.ClusterName,
		}).
		Output()
	if err != nil {
		return err
	}

	for _, instance := range strings.Split(out, "\n") {
		if instance == "" {
			continue
		}

		nameZone := strings.Split(instance, ",")
		if len(nameZone) != 2 {
			return fmt.Errorf("instance %s could not be parsed", instance)
		}

		name, zone := nameZone[0], nameZone[1]
		err := NewCommand(`gcloud -q compute ssh jenkins@{{.Name}} --project={{.GCloudProject}} --zone={{.Zone}} --command="sudo sysctl -w vm.max_map_count=262144"`).
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

func (d *GkeDriver) configureDocker() error {
	log.Println("Configuring Docker...")
	return NewCommand("gcloud auth configure-docker --quiet").Run()
}

func (d *GkeDriver) getCredentials() error {
	log.Println("Getting credentials...")
	return NewCommand("gcloud beta --project {{.GCloudProject}} container clusters get-credentials {{.ClusterName}} --region {{.Region}}").
		AsTemplate(map[string]interface{}{
			"GCloudProject": d.plan.Gke.GCloudProject,
			"ClusterName":   d.plan.ClusterName,
			"Region":        d.plan.Gke.Region,
		}).
		Run()
}

func (d *GkeDriver) delete() error {
	log.Println("Deleting cluster...")
	return NewCommand("gcloud beta --quiet --project {{.GCloudProject}} container clusters delete {{.ClusterName}} --region {{.Region}}").
		AsTemplate(map[string]interface{}{
			"GCloudProject": d.plan.Gke.GCloudProject,
			"ClusterName":   d.plan.ClusterName,
			"Region":        d.plan.Gke.Region,
		}).
		Run()
}
