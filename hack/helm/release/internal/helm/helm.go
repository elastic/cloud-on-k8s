// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package helm

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"github.com/avast/retry-go/v4"
	"google.golang.org/api/googleapi"
	"gopkg.in/yaml.v3"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/downloader"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/repo"
)

const (
	googleCredentialsEnvVar = "GOOGLE_APPLICATION_CREDENTIALS"
	defaultElasticHelmRepo  = "https://helm.elastic.co"
	chartYamlGlob           = "*/Chart.yaml"
	chartsSubdirYamlGlob    = "charts/*/Chart.yaml"
	stableHelmChartsURL     = "https://charts.helm.sh/stable"
)

// ReleaseConfig is the configuration needed to
// release all charts in a given directory.
type ReleaseConfig struct {
	// ChartsDir is the directory from which to release all Helm charts.
	ChartsDir string
	// Bucket is the GCS bucket to which to release all Helm charts.
	Bucket string
	// ChartsRepoURL is the Helm charts repository URL to which to release all Helm charts.
	ChartsRepoURL string
	// CredentialsFilePath is the path to the Google credentials JSON file.
	CredentialsFilePath string
	// DryRun determines whether to run as many operations as possible, without making any changes to
	// the GCS bucket, or the Helm repository index files.
	DryRun bool
	// Excludes is a slice of Helm chart names to ignore and not release.
	Excludes []string
}

// Release will run a Helm release process which consists of
// 1. Removing existing packaged Helm chart release files (tarballs)
// 2. Ensuring we have a Google credentials file for writing to GCS bucket.
// 3. Separating Helm charts with no dependencies from Helm charts with dependencies.
// 4. Uploading Helm charts with no dependencies to GCS bucket.
// 5. Potentially updating GCS Bucket Helm index.
// 6. Potentially uploading Helm charts with dependencies to GCS bucket.
// 7. Potentially updating GCS Bucket Helm index a second time.
func Release(conf ReleaseConfig) error {
	return internalRelease(conf)
}

func internalRelease(conf ReleaseConfig) error {
	log.Println("Removing existing releases.")
	if err := removeExistingReleases(conf.ChartsDir); err != nil {
		return err
	}
	if err := ensureCredentialsFile(conf.CredentialsFilePath); err != nil {
		return err
	}
	noDeps, withDeps, err := readAndSeparateChartsWithDependencies(conf.ChartsDir, conf.Excludes)
	if err != nil {
		return err
	}

	return uploadChartsAndUpdateIndex(uploadChartsConfig{
		releaseConf: conf,
		noDeps:      noDeps,
		withDeps:    withDeps,
	})
}

// removeExistingReleases will search for any existing Helm chart
// packaged releases in the form of tarballs (.tgz files)
// in the given Helm charts directory and remove them.
func removeExistingReleases(chartsDir string) error {
	files, _ := filepath.Glob(chartsDir + "*/*.tgz")
	for _, file := range files {
		log.Printf("removing existing release (%s)", file)
		if err := os.Remove(file); err != nil {
			return fmt.Errorf("while removing release file (%s): %w", file, err)
		}
	}
	return nil
}

// ensureCredentialsfile will ensure that the given path to a Google
// credentials file exists and is readable.
func ensureCredentialsFile(filePath string) error {
	_, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("while reading google credentials file (%s): %w", filePath, err)
	}
	return os.Setenv(googleCredentialsEnvVar, filePath)
}

// readAndSeparateChartsWithDependencies will read all files in the given Helm charts directory,
// excluding any names given in the excludes list, and generate a slice of Helm charts
// that has no *direct* dependencies,
// (dependencies with names that exist within the given Helm charts directory)
// and generate a slice of Helm charts with direct dependencies, return both with any errors.
func readAndSeparateChartsWithDependencies(chartsDir string, excludes []string) (charts, charts, error) {
	chs, err := readCharts(chartsDir, excludes)
	if err != nil {
		return nil, nil, err
	}
	noDeps, withDeps := separateChartsWithDependencies(chs)
	return noDeps, withDeps, nil
}

// separateChartsWithDependencies will separate the slice of charts into a slice
// of charts with no direct dependencies, and a slice of charts with direct
// dependencies, and return both.
func separateChartsWithDependencies(chs []chart) (noDeps, withDeps charts) {
	var temp []chart
	for _, ch := range chs {
		if len(ch.Dependencies) == 0 {
			noDeps = append(noDeps, ch)
			continue
		}
		temp = append(temp, ch)
	}
	for _, ch := range temp {
		foundInDeps := false
		for _, dep := range ch.Dependencies {
			if contains(dep.Name, noDeps.chartNames()) {
				withDeps = append(withDeps, ch)
				foundInDeps = true
				break
			}
		}
		if !foundInDeps {
			noDeps = append(noDeps, ch)
		}
	}
	return noDeps, withDeps
}

// readCharts will search for any Helm charts in the given Helm charts directory,
// read the Chart.yaml file, Unmarshaling it into a chart struct, ignoring
// any Helm charts with names from the excludes list, and return a slice of charts
// and any errors.
func readCharts(chartsDir string, excludes []string) ([]chart, error) {
	cs, err := filepath.Glob(filepath.Join(chartsDir, chartYamlGlob))
	if err != nil {
		return nil, fmt.Errorf("while searching for files matching pattern (%s): %w", chartYamlGlob, err)
	}
	// also check if the current directory given is a chart for the case of providing
	// the releaser a full path to a single chart to process.
	currentDirectoryCharts, err := filepath.Glob(filepath.Join(chartsDir, "Chart.yaml"))
	if err != nil {
		return nil, fmt.Errorf("while searching for files matching pattern (%s): %w", "Chart.yaml", err)
	}
	// also check if the current directory given has a sub-directory 'charts' that contains directories
	// with charts when the releaser is given a full path to a single chart directory to process.
	// (Glob doesn't support '**' unfortunately)
	currentDirectorySubCharts, err := filepath.Glob(filepath.Join(chartsDir, chartsSubdirYamlGlob))
	if err != nil {
		return nil, fmt.Errorf("while searching for files matching pattern (%s): %w", "Chart.yaml", err)
	}

	var charts []chart
	for _, fullChartPath := range append(cs, append(currentDirectoryCharts, currentDirectorySubCharts...)...) {
		fileBytes, err := os.ReadFile(fullChartPath)
		if err != nil {
			return nil, fmt.Errorf("while reading (%s): %w", fullChartPath, err)
		}
		var ch chart
		if err = yaml.Unmarshal(fileBytes, &ch); err != nil {
			return nil, fmt.Errorf("while unmarshaling (%s) to chart: %w", fullChartPath, err)
		}
		if contains(ch.Name, excludes) {
			log.Printf("Excluding (%s) as it is in the excludes list", ch.Name)
			continue
		}
		ch.fullPath = filepath.Dir(fullChartPath)
		charts = append(charts, ch)
	}

	return charts, nil
}

// contains is a helper to determine if a given string is in a slice of strings.
func contains(name string, names []string) bool {
	for _, n := range names {
		if n == name {
			return true
		}
	}
	return false
}

// uploadChartsConfig is the configuration for uploading Helm charts
// and updating the GCS bucket Helm index file.
type uploadChartsConfig struct {
	releaseConf ReleaseConfig
	noDeps      charts
	withDeps    charts
}

// uploadChartsAndUpdateIndex will perform the following:
// 1. Upload Helm charts with no dependencies to GCS bucket.
// 2. Potentially update GCS Bucket Helm index.
// 3. Potentially upload Helm charts with dependencies to GCS bucket.
// 4. Potentially update GCS Bucket Helm index a second time.
func uploadChartsAndUpdateIndex(conf uploadChartsConfig) error {

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	tempDir, err := os.MkdirTemp(os.TempDir(), "charts")
	if err != nil {
		return fmt.Errorf("while creating temporary directory for charts: %w", err)
	}
	defer os.RemoveAll(tempDir)
	// upload charts without dependencies
	if err := uploadCharts(ctx, tempDir, conf.noDeps, conf.releaseConf); err != nil {
		return err
	}
	if err := updateIndex(ctx, tempDir, conf); err != nil {
		return err
	}
	// This retry is here because of caching in front of the Helm repository
	// and the time it takes for a new release to show up in the repository.
	// If the eck-stack chart depends on new version of any of the other
	// eck-resources charts, and that new version is just released, then
	// it will take ~ one hour for it to show up, so we will continue trying
	// to get all dependencies of the helm charts, and upload them for 1 hour.
	retry.Do(
		func() error {
			if err := updateHelmRepositories(); err != nil {
				return err
			}
			// upload charts with dependencies
			if err := uploadCharts(ctx, tempDir, conf.withDeps, conf.releaseConf); err != nil {
				return err
			}
			if err := updateIndex(ctx, tempDir, conf); err != nil {
				return err
			}

			return nil
		},
		retry.RetryIf(func(err error) bool {
			if strings.Contains(err.Error(), "while updating dependencies for helm chart") && !conf.releaseConf.DryRun {
				return true
			}
			return false
		}),
		retry.Attempts(60),
		retry.Delay(1*time.Minute),
		retry.OnRetry(func(n uint, err error) {
			log.Printf("retry #%d: %s\n", n, err)
		}),
	)

	return nil
}

func updateHelmRepositories() error {
	// simulates helm repo update
	f, err := repo.LoadFile(cli.New().RepositoryConfig)
	if err != nil {
		return fmt.Errorf("while loading repository file: %w", err)
	}
	for _, r := range f.Repositories {
		cr, err := repo.NewChartRepository(r, getter.All(cli.New()))
		if err != nil {
			return fmt.Errorf("while getting new chart repository: %w", err)
		}
		if _, err = cr.DownloadIndexFile(); err != nil {
			return fmt.Errorf("while updating repository index for (%s): %w", cr.Config.Name, err)
		}
		log.Printf("Updated repository index for (%s)\n", cr.Config.Name)
	}
	return nil
}

// uploadCharts will perform the following:
//  1. Ensure that the Helm "stable" repository exists in the repository list.
//  2. For each Helm chart, copy the chart to a temporary directory
//  3. For each Helm chart's dependency, if the dependency's repository
//     is set to 'https://helm.elastic.co', and the flag 'charts-repo-url'
//     is set to another repository (such as a dev helm repo), the dependency's
//     repository will be re-written to the given 'charts-repo-url' flag.
//  4. Run 'Helm dependency update' for the Helm Chart.
//  5. Run 'Helm package chart' for the Helm Chart to generate a tarball.
//  6. Copy the tarball to the GCS bucket.
func uploadCharts(ctx context.Context, tempDir string, charts []chart, conf ReleaseConfig) error {
	type helmRepo struct {
		name, url string
	}
	for _, r := range []helmRepo{{name: "stable", url: stableHelmChartsURL}, {name: conf.Bucket, url: conf.ChartsRepoURL}} {
		if err := addHelmRepository(r.name, r.url); err != nil {
			return err
		}
	}
	log.Printf("Done adding (%s) repository.\n", stableHelmChartsURL)
	for _, chart := range charts {
		chartPath := filepath.Join(tempDir, chart.Name)
		if err := copyChartToTemporaryDirectory(chart, chartPath); err != nil {
			return err
		}
		log.Printf("Attempting to add chart (%s)\n", chart.Name)
		if err := updateDependencyChartURLs(chartPath, conf.ChartsRepoURL); err != nil {
			return err
		}
		if err := runChartDependencyUpdate(chartPath, chart.Name); err != nil {
			return err
		}
		if err := packageHelmChart(chartPath, chart.Name); err != nil {
			return err
		}
		source := fmt.Sprintf("%s-*.tgz", chart.Name)
		if err := copyChartToGCSBucket(ctx, copyChartToBucketConfig{
			bucket:        conf.Bucket,
			chartName:     chart.Name,
			tempDirectory: tempDir,
			sourceGlob:    source,
			dryRun:        conf.DryRun,
		}); err != nil {
			return err
		}
	}

	return nil
}

// addHelmRepository will add the given Helm repository name + url
// to the local Helm configuration.
func addHelmRepository(name, url string) error {
	settings := cli.New()
	log.Printf("Adding (%s) repository.\n", stableHelmChartsURL)
	// simulates 'helm repo add' command.
	if _, err := repo.NewChartRepository(&repo.Entry{
		Name: "stable",
		URL:  stableHelmChartsURL,
	}, getter.All(settings)); err != nil {
		return fmt.Errorf("while adding helm stable charts repository: %w", err)
	}
	log.Printf("Done adding (%s) repository.\n", stableHelmChartsURL)
	return nil
}

// copyChartToTemporaryDirectory will copy a given Helm chart
// to a given chartPath temporary directory.
func copyChartToTemporaryDirectory(ch chart, chartPath string) error {
	err := os.Mkdir(chartPath, 0755)
	if err != nil {
		return fmt.Errorf("while making chart directory inside temporary directory (%s): %w", chartPath, err)
	}
	err = copy(ch.fullPath, chartPath)
	if err != nil {
		return fmt.Errorf("while copying chart (%s) to temporary directory: %w", ch.Name, err)
	}
	return nil
}

// runChartDependencyUpdate runs 'helm dependency update chart' for a given Helm chart.
func runChartDependencyUpdate(chartPath, chartName string) error {
	settings := cli.New()
	client := action.NewDependency()
	cfg := action.Configuration{}
	man := &downloader.Manager{
		Out:              os.Stdout,
		ChartPath:        chartPath,
		Keyring:          client.Keyring,
		SkipUpdate:       client.SkipRefresh,
		Getters:          getter.All(settings),
		RegistryClient:   cfg.RegistryClient,
		RepositoryConfig: settings.RepositoryConfig,
		RepositoryCache:  settings.RepositoryCache,
		Debug:            settings.Debug,
		Verify:           downloader.VerifyNever,
	}
	if client.Verify {
		man.Verify = downloader.VerifyAlways
	}
	log.Printf("Updating dependencies for chart (%s)\n", chartName)
	if err := man.Update(); err != nil {
		return fmt.Errorf("while updating dependencies for helm chart (%s): %w", chartName, err)
	}
	return nil
}

// packageHelmChart runs 'helm package chart' for a given Helm chart.
func packageHelmChart(chartPath, chartName string) error {
	packageClient := action.NewPackage()
	log.Printf("Packaging chart (%s)\n", chartName)
	if _, err := packageClient.Run(chartPath, map[string]interface{}{}); err != nil {
		return fmt.Errorf("while packaging helm chart (%s): %w", chartName, err)
	}
	return nil
}

// copyChartToBucketConfig is the configuration for copying
// a chart to a GCS bucket.
type copyChartToBucketConfig struct {
	bucket, chartName, tempDirectory, sourceGlob string
	dryRun                                       bool
}

// copyChartToGCSBucket will potentially copy a Helm chart to a GCS bucket.
// If the object already exists within the bucket, it will not be overwritten.
func copyChartToGCSBucket(ctx context.Context, config copyChartToBucketConfig) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	storageClient, err := storage.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("while creating gcs storage client: %w", err)
	}
	defer storageClient.Close()

	files, err := filepath.Glob(config.sourceGlob)
	if err != nil {
		return fmt.Errorf("while search for file glob (%s): %w", config.sourceGlob, err)
	}
	if len(files) == 0 {
		return fmt.Errorf("couldn't file helm package with glob (%s)", config.sourceGlob)
	}
	// TODO is this 'helm' prefix always assumed?  I don't think so...
	// This should be the trailing path of the given repo url, right?
	destination := fmt.Sprintf("helm/%s/%s", config.chartName, files[0])
	log.Printf("Writing chart to bucket path (%s) \n", destination)
	f, err := os.Open(files[0])
	if err != nil {
		return fmt.Errorf("while opening chart (%s): %w", files[0], err)
	}
	defer f.Close()
	defer func() {
		// intentionally ignoring failure to remove temporary *.tgz file
		_ = os.Remove(files[0])
	}()

	if config.dryRun {
		log.Printf("not uploading (%s) as dry-run is set", files[0])
		return nil
	}

	bkt := storageClient.Bucket(config.bucket)

	o := bkt.Object(destination)

	// For an object that does not yet exist, set the DoesNotExist precondition,
	// to fail with an http error code '412' if the object already exists.
	o = o.If(storage.Conditions{DoesNotExist: true})

	// Upload an object with storage.Writer.
	wc := o.NewWriter(ctx)
	if _, err = io.Copy(wc, f); err != nil {
		return fmt.Errorf("while copying data to bucket: %w", err)
	}

	if err := wc.Close(); err != nil {
		switch ee := err.(type) {
		case *googleapi.Error:
			if ee.Code == http.StatusPreconditionFailed {
				// The object already exists; this error is expected
				break
			}
			return fmt.Errorf("while writing data to bucket: %w", err)
		default:
			return fmt.Errorf("while writing data to bucket: %w", err)
		}
	}
	data, err := ioutil.ReadFile(files[0])
	if err != nil {
		return fmt.Errorf("while reading (%s): %w", files[0], err)
	}

	err = ioutil.WriteFile(filepath.Join(config.tempDirectory, config.chartName, files[0]), data, 0777)
	if err != nil {
		return fmt.Errorf("while writing (%s) to temp directory: %w", files[0], err)
	}
	return nil
}

// copy will recursively copy a given source, to a given destination.
func copy(source, destination string) error {
	var err error = filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		var relPath string = strings.Replace(path, source, "", 1)
		if relPath == "" {
			return nil
		}
		if info.IsDir() {
			return os.Mkdir(filepath.Join(destination, relPath), 0755)
		} else {
			var data, err = ioutil.ReadFile(filepath.Join(source, relPath))
			if err != nil {
				return err
			}
			return ioutil.WriteFile(filepath.Join(destination, relPath), data, 0777)
		}
	})
	return err
}

// updateDependencyChartURLS will process a Helm chart's dependencies, and if the
// dependency's repository is set to 'https://helm.elastic.co', and the
// 'repoURL' is set to another repository (such as a dev helm repo), the dependency's
// repository will be re-written to the given 'repoURL' flag.
func updateDependencyChartURLs(chartPath string, repoURL string) error {
	chartYamlFilePath := filepath.Join(chartPath, "Chart.yaml")
	data, err := ioutil.ReadFile(chartYamlFilePath)
	if err != nil {
		return fmt.Errorf("while reading file (%s): %w", chartYamlFilePath, err)
	}
	u, err := url.Parse(repoURL)
	if err != nil {
		return fmt.Errorf("while parsing url (%s): %w", repoURL, err)
	}
	// repoURL potentially has trailing path (https://helm-dev.elastic.co/helm)
	// this strips the trailing path, and contains only the scheme://host
	url := fmt.Sprintf("%s://%s", u.Scheme, u.Host)
	newContents := strings.ReplaceAll(
		string(data),
		fmt.Sprintf(`repository: "%s"`, defaultElasticHelmRepo),
		fmt.Sprintf(`repository: "%s"`, url),
	)

	err = ioutil.WriteFile(chartYamlFilePath, []byte(newContents), 0)
	if err != nil {
		return fmt.Errorf("while writing (%s): %w", chartYamlFilePath, err)
	}
	return nil
}

// updateIndex will perform the following tasks:
// 1. Create a temporary index.yaml.old file.
// 2. Write the data from GCS bucket/index.yaml to tempDir/index.yaml.old.
// 3. Index the charts in tempDir, creating a new index.yaml file.
// 4. Merge the new index with the index from the GCS bucket.
// 5. Potentially write this new index.yaml file to GCS bucket/index.yaml.
func updateIndex(ctx context.Context, tempDir string, conf uploadChartsConfig) error {
	storageClient, err := storage.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("while creating gcs storage client: %w", err)
	}
	defer storageClient.Close()

	existingIndexFile := filepath.Join(tempDir, "index.yaml.old")
	f, err := os.Create(existingIndexFile)
	if err != nil {
		return fmt.Errorf("while creating empty index.yaml: %w", err)
	}

	reader, err := storageClient.Bucket(conf.releaseConf.Bucket).Object("index.yaml").NewReader(ctx)
	if err != nil {
		return fmt.Errorf("while creating new reader for index.yaml: %w", err)
	}
	defer reader.Close()

	if _, err := io.Copy(f, reader); err != nil {
		return fmt.Errorf("while writing index.yaml.old: %w", err)
	}

	if err = f.Close(); err != nil {
		return fmt.Errorf("while closing index.yaml.old file: %w", err)
	}

	// helm repo index --merge index.yaml.old --url chart_repo_url temp_charts_location
	tempIndex, err := repo.IndexDirectory(tempDir, conf.releaseConf.ChartsRepoURL)
	if err != nil {
		return fmt.Errorf("while indexing helm charts in temporary directory: %w", err)
	}

	bucketIndex, err := repo.LoadIndexFile(existingIndexFile)
	if err != nil {
		return fmt.Errorf("while loading existing helm index file: %w", err)
	}

	tempIndex.Merge(bucketIndex)
	tempIndex.SortEntries()

	if conf.releaseConf.DryRun {
		log.Printf("not uploading index as dry-run is set")
		return nil
	}

	log.Printf("Writing new helm index file for %s", conf.releaseConf.ChartsRepoURL)
	if err = tempIndex.WriteFile(filepath.Join(tempDir, "index.yaml"), 0644); err != nil {
		return fmt.Errorf("while writing new helm index file: %w", err)
	}
	f, err = os.Open(filepath.Join(tempDir, "index.yaml"))
	if err != nil {
		return fmt.Errorf("while opening new index.yaml: %w", err)
	}
	defer f.Close()

	writer := storageClient.Bucket(conf.releaseConf.Bucket).Object("index.yaml").NewWriter(ctx)

	if _, err = io.Copy(writer, f); err != nil {
		return fmt.Errorf("while copying new index.yaml to bucket: %w", err)
	}

	if err := writer.Close(); err != nil {
		return fmt.Errorf("while writing new index.yaml to bucket: %w", err)
	}
	return nil
}
