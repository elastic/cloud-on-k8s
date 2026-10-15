// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package runner

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/elastic/cloud-on-k8s/v3/hack/deployer/runner/bucket"
)

type fakeClusterContext struct {
	regionName        string
	existsValue       bool
	existsErr         error
	existsCalls       int
	createCalls       int
	createOutput      string
	createErr         error
	updateLabelsCalls int
	updateLabelsErr   error
	deleteCalls       int
	deleteErr         error
	credentialsCalls  int
	credentialsErr    error
	bindRolesCalls    int
	copyStorageCalls  int
	listCalls         int
	listResult        []string
	listErr           error
	clusterNames      []string
	projectName       string
}

func (f *fakeClusterContext) region() string         { return f.regionName }
func (f *fakeClusterContext) values() map[string]any { return map[string]any{"Region": f.regionName} }
func (f *fakeClusterContext) exists() (bool, error) {
	f.existsCalls++
	return f.existsValue, f.existsErr
}
func (f *fakeClusterContext) create(Plan) (string, error) {
	f.createCalls++
	return f.createOutput, f.createErr
}
func (f *fakeClusterContext) updateLabels() error {
	f.updateLabelsCalls++
	return f.updateLabelsErr
}
func (f *fakeClusterContext) delete() error {
	f.deleteCalls++
	return f.deleteErr
}
func (f *fakeClusterContext) bindRolesCmd() (string, error) {
	return "kubectl create clusterrolebinding ...", nil
}
func (f *fakeClusterContext) bindRoles() error {
	f.bindRolesCalls++
	return nil
}
func (f *fakeClusterContext) copyBuiltInStorageClasses() error {
	f.copyStorageCalls++
	return nil
}
func (f *fakeClusterContext) getCredentials() error {
	f.credentialsCalls++
	return f.credentialsErr
}
func (f *fakeClusterContext) listClusters(string, time.Time) ([]string, error) {
	f.listCalls++
	return f.listResult, f.listErr
}

// Unlike the production context, these methods return the same fake so tests can
// record derived values and assert all calls through one shared object.
func (f *fakeClusterContext) withClusterName(name string) clusterContext {
	f.clusterNames = append(f.clusterNames, name)
	return f
}
func (f *fakeClusterContext) withProject(project string) clusterContext {
	f.projectName = project
	return f
}

type fakeGKEBucketManager struct {
	deleteCalls int
	deleteErr   error
}

func (f *fakeGKEBucketManager) Create() error { return nil }
func (f *fakeGKEBucketManager) Delete() error {
	f.deleteCalls++
	return f.deleteErr
}

func TestGKELocalSSDOption(t *testing.T) {
	tests := []struct {
		name     string
		settings *GKESettings
		want     string
		wantErr  string
	}{
		{
			name:     "no local SSD",
			settings: &GKESettings{},
		},
		{
			name: "deprecated legacy local SSD compatibility",
			settings: &GKESettings{
				LocalSsdCount: 1,
			},
			want: "--local-ssd-count 1",
		},
		{
			name: "raw NVMe local SSD",
			settings: &GKESettings{
				LocalNvmeSsdBlock: true,
			},
			want: "--local-nvme-ssd-block count=1",
		},
		{
			name: "mutually exclusive modes",
			settings: &GKESettings{
				LocalSsdCount:     1,
				LocalNvmeSsdBlock: true,
			},
			wantErr: "localSsdCount and localNvmeSsdBlock are mutually exclusive",
		},
		{
			name: "negative count",
			settings: &GKESettings{
				LocalSsdCount: -1,
			},
			wantErr: "local SSD count must not be negative",
		},
		{
			name: "negative count with raw NVMe local SSD",
			settings: &GKESettings{
				LocalSsdCount:     -1,
				LocalNvmeSsdBlock: true,
			},
			wantErr: "local SSD count must not be negative",
		},
		{
			name:    "missing settings",
			wantErr: "GKE settings must be configured",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := gkeLocalSSDOption(tt.settings)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestGKECIPlanUsesRawNVMeLocalSSD(t *testing.T) {
	plansYAML, err := os.ReadFile("../config/plans.yml")
	require.NoError(t, err)

	var plans Plans
	require.NoError(t, yaml.Unmarshal(plansYAML, &plans))

	plan, err := choosePlan(plans.Plans, "gke-ci")
	require.NoError(t, err)
	require.Equal(t, "europe-west1", plan.Gke.Region)
	require.Equal(t, "n2d-standard-4", plan.MachineType)
	require.Zero(t, plan.Gke.LocalSsdCount)
	require.True(t, plan.Gke.LocalNvmeSsdBlock)
	require.Empty(t, plan.DiskSetup)

	option, err := gkeLocalSSDOption(plan.Gke)
	require.NoError(t, err)
	require.Equal(t, "--local-nvme-ssd-block count=1", option)

	plan = configureGKELocalSSD(plan)
	require.Equal(t, gkeLocalDiskSetupCommand, plan.DiskSetup)
}

func TestGKERegionsToTry(t *testing.T) {
	tests := []struct {
		name            string
		region          string
		fallbackRegions []string
		want            []string
	}{
		{
			name:   "no fallbacks",
			region: "europe-west1",
			want:   []string{"europe-west1"},
		},
		{
			name:            "fallbacks without duplicates",
			region:          "europe-west1",
			fallbackRegions: []string{"europe-west4", "us-east4"},
			want:            []string{"europe-west1", "europe-west4", "us-east4"},
		},
		{
			name:            "fallbacks containing the primary region",
			region:          "europe-west1",
			fallbackRegions: []string{"europe-west1", "us-east4"},
			want:            []string{"europe-west1", "us-east4"},
		},
		{
			name:            "fallbacks with repeated entries",
			region:          "europe-west1",
			fallbackRegions: []string{"us-east4", "us-east4"},
			want:            []string{"europe-west1", "us-east4"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, gkeRegions(tt.region, tt.fallbackRegions))
		})
	}
}

func TestGKEIsCapacityError(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   bool
	}{
		{name: "capacity", output: "ERROR: " + gkeCapacityErrorIndicator, want: true},
		{name: "quota", output: "ERROR: " + gkeQuotaErrorIndicator, want: true},
		{name: "permission", output: "ERROR: permission denied"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, isCapacityError(tt.output))
		})
	}
}

func TestGKEGetCredentialsFromClusters(t *testing.T) {
	primaryErr := errors.New("primary credentials failed")
	fallbackErr := errors.New("fallback credentials failed")
	tests := []struct {
		name       string
		errors     []error
		wantCalls  []int
		wantErrs   []error
		wantErrMsg string
	}{
		{
			name:       "no configured regions",
			wantErrMsg: "no regions configured",
		},
		{
			name:      "primary succeeds",
			errors:    []error{nil, nil},
			wantCalls: []int{1, 0},
		},
		{
			name:      "fallback succeeds",
			errors:    []error{primaryErr, nil},
			wantCalls: []int{1, 1},
		},
		{
			name:       "all regions fail",
			errors:     []error{primaryErr, fallbackErr},
			wantCalls:  []int{1, 1},
			wantErrs:   []error{primaryErr, fallbackErr},
			wantErrMsg: "failed to get credentials from configured regions",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakes := make([]*fakeClusterContext, len(tt.errors))
			clusters := make([]clusterContext, len(tt.errors))
			for i, credentialErr := range tt.errors {
				fake := &fakeClusterContext{
					regionName:     []string{"europe-west1", "europe-west4"}[i],
					credentialsErr: credentialErr,
				}
				fakes[i] = fake
				clusters[i] = fake
			}

			err := getCredentialsFromClusters(clusters)
			if tt.wantErrMsg == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tt.wantErrMsg)
			}
			for _, wantErr := range tt.wantErrs {
				require.ErrorIs(t, err, wantErr)
			}
			for i, wantCalls := range tt.wantCalls {
				require.Equal(t, wantCalls, fakes[i].credentialsCalls)
			}
		})
	}
}

func TestGKEDriverCreate(t *testing.T) {
	credentialsErr := errors.New("stop after credentials")
	existsErr := errors.New("failed to query GKE")
	updateErr := errors.New("label update failed")

	tests := []struct {
		name      string
		plan      Plan
		fakes     []*fakeClusterContext
		wantErr   error
		configure func(*GKEDriver)
		check     func(*testing.T, []*fakeClusterContext)
	}{
		{
			name:  "successful standard cluster creation",
			plan:  Plan{Gke: &GKESettings{}},
			fakes: []*fakeClusterContext{{regionName: "europe-west1"}},
			configure: func(d *GKEDriver) {
				d.createStorageClassFn = func() error { return nil }
			},
			check: func(t *testing.T, fakes []*fakeClusterContext) {
				t.Helper()
				require.Equal(t, 1, fakes[0].existsCalls)
				require.Equal(t, 1, fakes[0].createCalls)
				require.Equal(t, 1, fakes[0].bindRolesCalls)
				require.Equal(t, 1, fakes[0].credentialsCalls)
				require.Equal(t, 1, fakes[0].copyStorageCalls)
			},
		},
		{
			name: "capacity failure falls back and finishes fallback cluster",
			plan: Plan{Gke: &GKESettings{}},
			fakes: []*fakeClusterContext{
				{
					regionName:   "europe-west1",
					createOutput: gkeCapacityErrorIndicator,
					createErr:    errors.New("capacity exhausted"),
				},
				{regionName: "europe-west4"},
			},
			configure: func(d *GKEDriver) {
				d.createStorageClassFn = func() error { return nil }
			},
			check: func(t *testing.T, fakes []*fakeClusterContext) {
				t.Helper()
				require.Equal(t, 1, fakes[0].createCalls)
				require.Equal(t, 1, fakes[0].deleteCalls)
				require.Zero(t, fakes[0].bindRolesCalls)
				require.Zero(t, fakes[0].credentialsCalls)
				require.Equal(t, 1, fakes[1].createCalls)
				require.Equal(t, 1, fakes[1].bindRolesCalls)
				require.Equal(t, 1, fakes[1].credentialsCalls)
				require.Equal(t, 1, fakes[1].copyStorageCalls)
			},
		},
		{
			name: "finds existing cluster in fallback region",
			plan: Plan{ClusterName: "existing-cluster", Gke: &GKESettings{}},
			fakes: []*fakeClusterContext{
				{regionName: "europe-west1"},
				{regionName: "europe-west4", existsValue: true, credentialsErr: credentialsErr},
				{regionName: "us-east4"},
			},
			wantErr: credentialsErr,
			check: func(t *testing.T, fakes []*fakeClusterContext) {
				t.Helper()
				require.Equal(t, 1, fakes[0].existsCalls)
				require.Equal(t, 1, fakes[1].existsCalls)
				require.Zero(t, fakes[2].existsCalls)
				require.Zero(t, fakes[0].createCalls+fakes[1].createCalls+fakes[2].createCalls)
				require.Equal(t, 1, fakes[1].credentialsCalls)
				require.Zero(t, fakes[1].bindRolesCalls)
			},
		},
		{
			name: "existing cluster recovered from interrupted run is fully configured",
			plan: Plan{ClusterName: "existing-cluster", Gke: &GKESettings{}},
			fakes: []*fakeClusterContext{
				{regionName: "europe-west1"},
				{regionName: "europe-west4", existsValue: true},
				{regionName: "us-east4"},
			},
			configure: func(d *GKEDriver) {
				d.createStorageClassFn = func() error { return nil }
			},
			check: func(t *testing.T, fakes []*fakeClusterContext) {
				t.Helper()
				require.Zero(t, fakes[0].createCalls+fakes[1].createCalls+fakes[2].createCalls)
				require.Equal(t, 1, fakes[1].credentialsCalls)
				require.Equal(t, 1, fakes[1].bindRolesCalls)
				require.Equal(t, 1, fakes[1].copyStorageCalls)
			},
		},
		{
			name: "stops when existing cluster check fails",
			plan: Plan{Gke: &GKESettings{Private: true}},
			fakes: []*fakeClusterContext{
				{regionName: "europe-west1", existsErr: existsErr},
				{regionName: "europe-west4"},
			},
			wantErr: existsErr,
			check: func(t *testing.T, fakes []*fakeClusterContext) {
				t.Helper()
				require.Equal(t, 1, fakes[0].existsCalls)
				require.Zero(t, fakes[1].existsCalls)
				require.Zero(t, fakes[0].createCalls+fakes[1].createCalls)
			},
		},
		{
			name: "Autopilot label failure does not delete cluster",
			plan: Plan{Gke: &GKESettings{Autopilot: true}},
			fakes: []*fakeClusterContext{
				{regionName: "europe-west1", updateLabelsErr: updateErr},
				{regionName: "europe-west4"},
			},
			wantErr: updateErr,
			check: func(t *testing.T, fakes []*fakeClusterContext) {
				t.Helper()
				require.Equal(t, 1, fakes[0].createCalls)
				require.Equal(t, 1, fakes[0].updateLabelsCalls)
				require.Zero(t, fakes[0].deleteCalls)
				require.Zero(t, fakes[1].createCalls)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clusters := make([]clusterContext, len(tt.fakes))
			for i := range tt.fakes {
				clusters[i] = tt.fakes[i]
			}
			d := &GKEDriver{plan: tt.plan, clusters: clusters}
			if tt.configure != nil {
				tt.configure(d)
			}
			err := d.create()
			if tt.wantErr == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tt.wantErr)
			}
			tt.check(t, tt.fakes)
		})
	}
}

func TestGKEDriverDelete(t *testing.T) {
	clusterErr := errors.New("cluster delete failed")
	bucketErr := errors.New("bucket delete failed")
	tests := []struct {
		name       string
		clusterErr error
		bucketErr  error
		wantErrs   []error
	}{
		{
			name: "attempts every region",
		},
		{
			name:       "continues and joins errors",
			clusterErr: clusterErr,
			bucketErr:  bucketErr,
			wantErrs:   []error{clusterErr, bucketErr},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			primary := &fakeClusterContext{regionName: "europe-west1", deleteErr: tt.clusterErr}
			fallback := &fakeClusterContext{regionName: "europe-west4"}
			bucketManager := &fakeGKEBucketManager{deleteErr: tt.bucketErr}
			var bucketRegions []string
			d := &GKEDriver{
				plan:     Plan{Bucket: &BucketSettings{}},
				clusters: []clusterContext{primary, fallback},
				newBucketManagerFn: func(_ Plan, c clusterContext) (bucket.Manager, error) {
					bucketRegions = append(bucketRegions, c.region())
					return bucketManager, nil
				},
			}

			err := d.delete()
			if len(tt.wantErrs) == 0 {
				require.NoError(t, err)
			}
			for _, wantErr := range tt.wantErrs {
				require.ErrorIs(t, err, wantErr)
			}
			require.Equal(t, []string{"europe-west1", "europe-west4"}, bucketRegions)
			require.Equal(t, 2, bucketManager.deleteCalls)
			require.Equal(t, 1, primary.deleteCalls)
			require.Equal(t, 1, fallback.deleteCalls)
		})
	}
}

func TestGKEDriverCleanup(t *testing.T) {
	listErr := errors.New("list failed")
	bucketErr := errors.New("bucket delete failed")
	tests := []struct {
		name      string
		listErr   error
		bucketErr error
		wantErrs  []error
	}{
		{
			name: "deletes cluster from fallback region",
		},
		{
			name:      "continues and joins errors",
			listErr:   listErr,
			bucketErr: bucketErr,
			wantErrs:  []error{listErr, bucketErr},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			primary := &fakeClusterContext{regionName: "europe-west1", listErr: tt.listErr}
			fallback := &fakeClusterContext{
				regionName: "europe-west4",
				listResult: []string{"old-cluster"},
			}
			bucketManager := &fakeGKEBucketManager{deleteErr: tt.bucketErr}
			var bucketRegions []string
			d := &GKEDriver{
				plan: Plan{
					Gke:    &GKESettings{GCloudProject: "test-project"},
					Bucket: &BucketSettings{},
				},
				clusters: []clusterContext{primary, fallback},
				newBucketManagerFn: func(_ Plan, c clusterContext) (bucket.Manager, error) {
					bucketRegions = append(bucketRegions, c.region())
					return bucketManager, nil
				},
			}

			err := d.cleanup("eck-e2e-", time.Hour)
			if len(tt.wantErrs) == 0 {
				require.NoError(t, err)
			}
			for _, wantErr := range tt.wantErrs {
				require.ErrorIs(t, err, wantErr)
			}
			require.Equal(t, 1, primary.listCalls)
			require.Equal(t, 1, fallback.listCalls)
			require.Equal(t, "test-project", primary.projectName)
			require.Equal(t, "test-project", fallback.projectName)
			require.Equal(t, []string{"old-cluster"}, fallback.clusterNames)
			require.Equal(t, 1, fallback.deleteCalls)
			require.Equal(t, []string{"europe-west4"}, bucketRegions)
		})
	}
}

func TestGKECreateWithFallback(t *testing.T) {
	someErr := errors.New("exit status 1")
	someDelErr := errors.New("delete exit status 1")
	capacityOutput := gkeCapacityErrorIndicator
	type createResponse struct {
		output string
		err    error
	}

	tests := []struct {
		name            string
		region          string
		fallbackRegions []string
		createResponses []createResponse
		deleteErr       error // returned by every delete call; nil means success
		noRegions       bool
		wantErr         bool
		wantErrMessage  string
		wantRegion      string
		wantCreateCalls int
		wantDeleteCalls int
	}{
		{
			name:           "no configured regions",
			noRegions:      true,
			wantErr:        true,
			wantErrMessage: "no GKE regions configured",
		},
		{
			name:            "success in primary region",
			region:          "europe-west1",
			createResponses: []createResponse{{}},
			wantRegion:      "europe-west1",
			wantCreateCalls: 1,
		},
		{
			name:            "primary failure is cleaned up before fallback succeeds",
			region:          "europe-west1",
			fallbackRegions: []string{"europe-west4"},
			createResponses: []createResponse{{capacityOutput, someErr}, {}},
			wantRegion:      "europe-west4",
			wantCreateCalls: 2,
			wantDeleteCalls: 1,
		},
		{
			name:            "all regions fail and are cleaned up",
			region:          "europe-west1",
			fallbackRegions: []string{"europe-west4", "us-east4"},
			createResponses: []createResponse{{capacityOutput, someErr}, {capacityOutput, someErr}, {capacityOutput, someErr}},
			wantErr:         true,
			wantRegion:      "europe-west1",
			wantCreateCalls: 3,
			wantDeleteCalls: 3,
		},
		{
			name:            "failure without fallback is cleaned up",
			region:          "europe-west1",
			createResponses: []createResponse{{capacityOutput, someErr}},
			wantErr:         true,
			wantRegion:      "europe-west1",
			wantCreateCalls: 1,
			wantDeleteCalls: 1,
		},
		{
			name:            "cleanup failure stops fallback",
			region:          "europe-west1",
			fallbackRegions: []string{"europe-west4"},
			createResponses: []createResponse{{capacityOutput, someErr}, {}},
			deleteErr:       someDelErr,
			wantErr:         true,
			wantRegion:      "europe-west1",
			wantCreateCalls: 1,
			wantDeleteCalls: 1,
		},
		{
			name:            "non-capacity failure is cleaned up without trying fallback",
			region:          "europe-west1",
			fallbackRegions: []string{"europe-west4"},
			createResponses: []createResponse{{"permission denied", someErr}},
			wantErr:         true,
			wantCreateCalls: 1,
			wantDeleteCalls: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			regions := gkeRegions(tt.region, tt.fallbackRegions)
			if tt.noRegions {
				regions = nil
			}
			clusters := make([]clusterContext, 0, len(regions))
			var fakes []*fakeClusterContext
			for i, region := range regions {
				response := createResponse{}
				if i < len(tt.createResponses) {
					response = tt.createResponses[i]
				}
				fake := &fakeClusterContext{
					regionName:   region,
					createOutput: response.output,
					createErr:    response.err,
					deleteErr:    tt.deleteErr,
				}
				fakes = append(fakes, fake)
				clusters = append(clusters, fake)
			}

			cluster, err := createInRegions(Plan{}, clusters)
			if tt.wantErr {
				require.Error(t, err)
				if tt.wantErrMessage != "" {
					require.ErrorContains(t, err, tt.wantErrMessage)
				}
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.wantRegion, cluster.region())
			}
			var createCalls, deleteCalls int
			for _, fake := range fakes {
				createCalls += fake.createCalls
				deleteCalls += fake.deleteCalls
			}
			require.Equal(t, tt.wantCreateCalls, createCalls)
			require.Equal(t, tt.wantDeleteCalls, deleteCalls)
		})
	}
}

func TestGKEDevPlanDefaultsToPersistentDiskWithLocalSSDOptIn(t *testing.T) {
	plansYAML, err := os.ReadFile("../config/plans.yml")
	require.NoError(t, err)

	var plans Plans
	require.NoError(t, yaml.Unmarshal(plansYAML, &plans))

	plan, err := choosePlan(plans.Plans, "gke-dev")
	require.NoError(t, err)
	require.Equal(t, "europe-west1", plan.Gke.Region)
	require.Equal(t, "n2d-standard-8", plan.MachineType)
	require.Zero(t, plan.Gke.LocalSsdCount)
	require.False(t, plan.Gke.LocalNvmeSsdBlock)
	require.Empty(t, plan.DiskSetup)

	option, err := gkeLocalSSDOption(plan.Gke)
	require.NoError(t, err)
	require.Empty(t, option)

	var runConfig RunConfig
	require.NoError(t, yaml.Unmarshal([]byte(`
id: gke-dev
overrides:
  gke:
    localNvmeSsdBlock: true
`), &runConfig))

	plan, err = GetPlan(plans.Plans, runConfig, "")
	require.NoError(t, err)
	require.True(t, plan.Gke.LocalNvmeSsdBlock)
	require.Empty(t, plan.DiskSetup)

	option, err = gkeLocalSSDOption(plan.Gke)
	require.NoError(t, err)
	require.Equal(t, "--local-nvme-ssd-block count=1", option)

	plan = configureGKELocalSSD(plan)
	require.Equal(t, gkeLocalDiskSetupCommand, plan.DiskSetup)
}
