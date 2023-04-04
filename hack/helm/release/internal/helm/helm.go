// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package helm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"github.com/avast/retry-go/v4"
	"golang.org/x/exp/slices"
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
	// KeepTempDir will retain the temporary directories.
	KeepTempDir bool
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
		if slices.Contains(excludes, ch.Name) {
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
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	tempDir, err := os.MkdirTemp(os.TempDir(), "charts_no_deps")
	if err != nil {
		return fmt.Errorf("while creating temporary directory for charts without dependencies: %w", err)
	}
	defer func() {
		if !conf.releaseConf.KeepTempDir {
			os.RemoveAll(tempDir)
		}
	}()
	log.Printf("temporary directory for charts without dependencies (%s)", tempDir)

	tempDirWithDeps, err := os.MkdirTemp(os.TempDir(), "charts_with_deps")
	if err != nil {
		return fmt.Errorf("while creating temporary directory for charts with dependencies: %w", err)
	}
	defer func() {
		if !conf.releaseConf.KeepTempDir {
			os.RemoveAll(tempDirWithDeps)
		}
	}()
	log.Printf("temporary directory for charts with dependencies (%s)", tempDirWithDeps)
	// This retry is here because of caching in front of the Helm repository
	// and the time it takes for a new release to show up in the repository.
	// If the eck-stack chart depends on new version of any of the other
	// eck-resources charts, and that new version is just released, then
	// it will take ~ one hour for it to show up, so we will continue trying
	// to get all dependencies of the helm charts, and upload them for 1 hour.
	err = retry.Do(
		func() error {
			// upload charts without dependencies
			if err := uploadCharts(ctx, tempDir, conf.noDeps, conf.releaseConf); err != nil {
				return err
			}
			var idx *index
			if idx, err = updateIndex(ctx, tempDir, conf, nil); err != nil {
				return err
			}

			// upload charts with dependencies
			if err := addDefaultHelmRepositories(conf.releaseConf); err != nil {
				return err
			}
			if err := updateHelmRepositories(); err != nil {
				return err
			}
			// upload charts with dependencies
			if err := uploadCharts(ctx, tempDirWithDeps, conf.withDeps, conf.releaseConf); err != nil {
				return err
			}
			if _, err := updateIndex(ctx, tempDirWithDeps, conf, idx); err != nil {
				return err
			}

			return nil
		},
		retry.RetryIf(func(err error) bool {
			if conf.releaseConf.DryRun {
				return false
			}
			if strings.Contains(err.Error(), "while updating dependencies for helm chart") {
				return true
			}
			var e *googleapi.Error
			if errors.As(err, &e) {
				return e.Code == http.StatusPreconditionFailed
			}
			return false
		}),
		retry.Attempts(60),
		retry.MaxJitter(30*time.Second),
		retry.DelayType(retry.RandomDelay),
		retry.OnRetry(func(n uint, err error) {
			log.Printf("retry #%d: %s\n", n, err)
		}),
	)
	if err != nil {
		return fmt.Errorf("while processing charts with dependencies: %w", err)
	}

	return nil
}

type helmRepo struct {
	name, url string
}

func addDefaultHelmRepositories(conf ReleaseConfig) error {
	for _, r := range []helmRepo{{name: "stable", url: stableHelmChartsURL}, {name: conf.Bucket, url: conf.ChartsRepoURL}} {
		if err := addHelmRepository(r.name, r.url); err != nil {
			return err
		}
	}
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
	if len(charts) == 0 {
		return nil
	}
	u, err := url.Parse(conf.ChartsRepoURL)
	if err != nil {
		return fmt.Errorf("while parsing url (%s): %w", conf.ChartsRepoURL, err)
	}
	for _, chart := range charts {
		chartPath := filepath.Join(tempDir, chart.Name)
		if err := copyChartToTemporaryDirectory(chart, chartPath); err != nil {
			return err
		}
		log.Printf("Attempting to add chart (%s)\n", chart.Name)
		if err := updateDependencyChartURLs(chartPath, u); err != nil {
			return err
		}
		if err := runChartDependencyUpdate(chartPath, chart.Name); err != nil {
			return err
		}
		if err := packageHelmChart(chartPath, chart.Name, tempDir); err != nil {
			return err
		}
		source := fmt.Sprintf("%s-*.tgz", chart.Name)
		if err := copyChartToGCSBucket(ctx, copyChartToBucketConfig{
			bucket:        conf.Bucket,
			chartName:     chart.Name,
			path:          u.Path,
			tempDirectory: tempDir,
			sourceGlob:    source,
			dryRun:        conf.DryRun,
			keepTempDir:   conf.KeepTempDir,
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
	// Ensure the configuration file's directory path exists
	err := os.MkdirAll(filepath.Dir(settings.RepositoryConfig), os.ModePerm)
	if err != nil && !os.IsExist(err) {
		return err
	}
	log.Printf("Adding (%s) repository.\n", name)
	// simulates 'helm repo add' command.
	b, err := os.ReadFile(settings.RepositoryConfig)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	var f repo.File
	if err := yaml.Unmarshal(b, &f); err != nil {
		return err
	}

	// return early if repository already exists
	if f.Has(name) {
		log.Printf("Done adding (%s) repository.\n", name)
		return nil
	}

	c := repo.Entry{
		Name: name,
		// The index (index.yaml) for both production and dev exist at '/', not '/helm'
		// which is where the helm data exists so trim the '/helm' suffix.
		URL: strings.TrimSuffix(url, "/helm"),
	}

	f.Update(&c)

	if err := f.WriteFile(settings.RepositoryConfig, 0644); err != nil {
		return err
	}
	log.Printf("Done adding (%s) repository.\n", name)
	return nil
}

// copyChartToTemporaryDirectory will copy a given Helm chart
// to a given chartPath temporary directory.
func copyChartToTemporaryDirectory(ch chart, chartPath string) error {
	// if the directory already exists in the temp directory
	// then this is a retry operation and unnecessary.
	if _, err := os.Stat(chartPath); !os.IsNotExist(err) {
		return nil
	}
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
func packageHelmChart(chartPath, chartName, tempDir string) error {
	packageClient := action.NewPackage()
	packageClient.Destination = filepath.Join(tempDir, chartName)
	log.Printf("Packaging chart (%s)\n", chartName)
	if _, err := packageClient.Run(chartPath, map[string]interface{}{}); err != nil {
		return fmt.Errorf("while packaging helm chart (%s): %w", chartName, err)
	}
	return nil
}

// copyChartToBucketConfig is the configuration for copying
// a chart to a GCS bucket.
type copyChartToBucketConfig struct {
	bucket, chartName, path, tempDirectory, sourceGlob string
	dryRun, keepTempDir                                bool
}

// copyChartToGCSBucket will potentially copy a Helm chart to a GCS bucket.
// If the object already exists within the bucket, it will not be overwritten.
func copyChartToGCSBucket(ctx context.Context, config copyChartToBucketConfig) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	storageClient, err := storage.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("while creating gcs storage client: %w", err)
	}
	defer storageClient.Close()

	files, err := filepath.Glob(filepath.Join(config.tempDirectory, "*", config.sourceGlob))
	if err != nil {
		return fmt.Errorf("while searching for file glob (%s): %w", config.sourceGlob, err)
	}
	if len(files) == 0 {
		return fmt.Errorf("couldn't find helm package with glob (%s)", config.sourceGlob)
	}
	destination := fmt.Sprintf("%s/%s/%s", strings.TrimPrefix(config.path, "/"), config.chartName, filepath.Base(files[0]))
	log.Printf("Writing chart to bucket path (%s) \n", destination)
	f, err := os.Open(files[0])
	if err != nil {
		return fmt.Errorf("while opening chart (%s): %w", files[0], err)
	}
	defer f.Close()

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
		var e *googleapi.Error
		if errors.As(err, &e) {
			if e.Code == http.StatusPreconditionFailed {
				// The object already exists; this error is expected
				return nil
			}
		}
		return fmt.Errorf("while writing data to bucket: %w", err)
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
		}
		var data []byte
		var readErr error
		data, readErr = os.ReadFile(filepath.Join(source, relPath))
		if readErr != nil {
			return err
		}
		return os.WriteFile(filepath.Join(destination, relPath), data, 0777)
	})
	return err
}

// updateDependencyChartURLS will process a Helm chart's dependencies, and if the
// dependency's repository is set to 'https://helm.elastic.co', and the
// 'repoURL' is set to another repository (such as a dev helm repo), the dependency's
// repository will be re-written to the given 'repoURL' flag.
func updateDependencyChartURLs(chartPath string, repoURL *url.URL) error {
	chartYamlFilePath := filepath.Join(chartPath, "Chart.yaml")
	data, err := os.ReadFile(chartYamlFilePath)
	if err != nil {
		return fmt.Errorf("while reading file (%s): %w", chartYamlFilePath, err)
	}
	// repoURL potentially has trailing path (https://helm-dev.elastic.co/helm)
	// this strips the trailing path, and contains only the scheme://host
	url := fmt.Sprintf("%s://%s", repoURL.Scheme, repoURL.Host)
	newContents := strings.ReplaceAll(
		string(data),
		fmt.Sprintf(`repository: "%s"`, defaultElasticHelmRepo),
		fmt.Sprintf(`repository: "%s"`, url),
	)

	err = os.WriteFile(chartYamlFilePath, []byte(newContents), 0)
	if err != nil {
		return fmt.Errorf("while writing (%s): %w", chartYamlFilePath, err)
	}
	return nil
}

// index is used internally to ensure we do not have to read the index from the google bucket
// multiple times and ensures that we maintain the proper generation/version of the file
// throughout this whole release process.
type index struct {
	// path is the local path to the previous index
	path string
	// generation is the generation/version of the previous index
	generation int64
}

// updateIndex will perform the following tasks:
// 1. Create a temporary index.yaml.old file.
// 2. Write the data from GCS bucket/index.yaml to tempDir/index.yaml.old.
// 3. Index the charts in tempDir, creating a new index.yaml file.
// 4. Merge the new index with the index from the GCS bucket.
// 5. Potentially write this new index.yaml file to GCS bucket/index.yaml.
func updateIndex(ctx context.Context, tempDir string, conf uploadChartsConfig, idx *index) (*index, error) {
	storageClient, err := storage.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("while creating gcs storage client: %w", err)
	}
	defer storageClient.Close()

	existingIndexFile := filepath.Join(tempDir, oldIndexFileName)
	if idx != nil {
		log.Printf("Reading previous index at generation (%d) from path (%s)", idx.generation, idx.path)
		b, err := os.ReadFile(idx.path)
		if err != nil {
			return nil, fmt.Errorf("while reading previous index file: %w", err)
		}
		err = os.WriteFile(existingIndexFile, b, 0600)
		if err != nil {
			return nil, fmt.Errorf("while writing previous index file: %w", err)
		}
	} else {
		idx, err = readIndexFromBucket(ctx, readIndexConfig{
			client:    storageClient,
			indexFile: existingIndexFile,
			bucket:    conf.releaseConf.Bucket,
		})
		if err != nil {
			return nil, err
		}
	}

	updatedIndexFile, err := indexTempDirAndMergeWithOldIndex(tempDir, conf.releaseConf.ChartsRepoURL, existingIndexFile)
	if err != nil {
		return nil, fmt.Errorf("while opening new %s: %w", indexFileName, err)
	}
	defer updatedIndexFile.Close()

	if conf.releaseConf.DryRun {
		log.Printf("not uploading index as dry-run is set")
		return nil, nil
	}

	return writeIndexToBucket(ctx, writeIndexConfig{
		client:          storageClient,
		bucketFileName:  indexFileName,
		bucket:          conf.releaseConf.Bucket,
		chartsRepoURL:   conf.releaseConf.ChartsRepoURL,
		existingIndex:   idx,
		indexFileHandle: updatedIndexFile,
	})
}

// indexTempDirAndMergeWithOldIndex will index the given tempdir, merge with the old existingIndexFile
// and write a new index file, returning the file handle and any errors encountered.
func indexTempDirAndMergeWithOldIndex(tempDir, chartsRepoURL, existingIndexFile string) (*os.File, error) {
	// helm repo index --merge index.yaml.old --url chart_repo_url temp_charts_location
	tempIndex, err := repo.IndexDirectory(tempDir, chartsRepoURL)
	if err != nil {
		return nil, fmt.Errorf("while indexing helm charts in temporary directory: %w", err)
	}

	bucketIndex, err := repo.LoadIndexFile(existingIndexFile)
	if err != nil {
		return nil, fmt.Errorf("while loading existing helm index file: %w", err)
	}

	tempIndex.Merge(bucketIndex)
	tempIndex.SortEntries()

	log.Printf("Writing new local helm index file for %s", chartsRepoURL)
	if err = tempIndex.WriteFile(filepath.Join(tempDir, indexFileName), 0644); err != nil {
		return nil, fmt.Errorf("while writing new helm index file: %w", err)
	}
	return os.Open(filepath.Join(tempDir, indexFileName))
}
