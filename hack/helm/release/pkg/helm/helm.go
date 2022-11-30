package helm

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"google.golang.org/api/option"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/downloader"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/repo"
)

const (
	googleCredentialsEnvVar = "GOOGLE_APPLICATION_CREDENTIALS"
	defaultGCSURL           = "https://storage.googleapis.com"
)

type ReleaseConfig struct {
	ChartsDir, Bucket, ChartsRepoURL string
	CredentialsFilePath              string
	UploadIndex                      bool
	UpdateDependencies               bool
	GCSURL                           string
}

func Release(conf ReleaseConfig) error {
	// remove existing releases
	log.Println("Removing existing releases.")
	if err := removeExistinReleases(conf.ChartsDir); err != nil {
		return err
	}
	if err := ensureCredentialsFile(conf.CredentialsFilePath); err != nil {
		return err
	}
	// upload charts
	if err := uploadCharts(conf.ChartsDir, conf.Bucket, conf.GCSURL, conf.UpdateDependencies); err != nil {
		return err
	}
	// update index
	if err := updateIndex(); err != nil {
		return err
	}
	return nil
}

func ensureCredentialsFile(path string) error {
	_, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("while reading google credentials file (%s): %w", path, err)
	}
	return os.Setenv(googleCredentialsEnvVar, path)
}

func removeExistinReleases(chartsDir string) error {
	// Cleanup existing releases
	files, _ := filepath.Glob(chartsDir + "*/*.tgz")
	for _, file := range files {
		log.Printf("removing file: %s", file)
		if err := os.Remove(file); err != nil {
			return fmt.Errorf("while removing file (%s): %w", file, err)
		}
	}
	return nil
}

func uploadCharts(chartsDir, bucket, gcsURL string, updateDeps bool) error {
	settings := cli.New()
	log.Println("Adding https://charts.helm.sh/stable repository.")
	// helm repo add stable https://charts.helm.sh/stable
	if _, err := repo.NewChartRepository(&repo.Entry{
		Name: "stable",
		URL:  "https://charts.helm.sh/stable",
	}, getter.All(settings)); err != nil {
		return fmt.Errorf("while adding helm repository: %w", err)
	}
	log.Println("Done Adding https://charts.helm.sh/stable repository.")
	charts, _ := filepath.Glob(filepath.Join(chartsDir, "*/Chart.yaml"))
	for _, chart := range charts {
		chartName := strings.Split(chart, "/")[len(strings.Split(chart, "/"))-2]
		chartPath := filepath.Join(chartsDir, chartName)
		log.Printf("Attempting to add chart (%s)\n", chartName)
		// helm dependency update charts_dir/chart
		if updateDeps {
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
			}
			if client.Verify {
				man.Verify = downloader.VerifyAlways
			}
			log.Printf("Updating dependencies for chart (%s)\n", chartName)
			if err := man.Update(); err != nil {
				return fmt.Errorf("while running 'helm dependency update %s': %w", chart, err)
			}
		}
		// helm package "charts_dir/chart" --destination "chart"
		packageClient := action.NewPackage()
		log.Printf("Packaging chart (%s)\n", chartName)
		if _, err := packageClient.Run(chartPath, map[string]interface{}{}); err != nil {
			return fmt.Errorf("while running 'helm package %s: %w", chartPath, err)
		}
		source := fmt.Sprintf("%s-*.tgz", chartName)
		destination := fmt.Sprintf("%s/helm/%s/", bucket, chartName)

		// gsutil cp -n source destination
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		log.Printf("Writing chart (%s) to bucket\n", chartName)
		storageClient, err := getGCSClient(ctx, gcsURL)
		if err != nil {
			return fmt.Errorf("while creating gcs storage client: %w", err)
		}
		defer storageClient.Close()

		files, err := filepath.Glob(source)
		if err != nil {
			return fmt.Errorf("while search for file glob (%s): %w", source, err)
		}
		if len(files) == 0 {
			return fmt.Errorf("couldn't file helm package with glob (%s)", source)
		}
		// Open Chart
		f, err := os.Open(files[0])
		if err != nil {
			return fmt.Errorf("while opening chart %s: %w", files[0], err)
		}
		defer f.Close()

		bkt := storageClient.Bucket(bucket)

		_, err = bkt.Attrs(ctx)
		if err != nil && !strings.Contains(err.Error(), "bucket doesn't exist") {
			return fmt.Errorf("while checking if bucket exists: %w", err)
		} else if err != nil && strings.Contains(err.Error(), "bucket doesn't exist") {
			// TODO get project id from json file
			if err := bkt.Create(ctx, "", nil); err != nil {
				return fmt.Errorf("while creating bucket: %w", err)
			}
		}

		o := bkt.Object(destination)

		// Optional: set a generation-match precondition to avoid potential race
		// conditions and data corruptions. The request to upload is aborted if the
		// object's generation number does not match your precondition.
		// For an object that does not yet exist, set the DoesNotExist precondition.
		o = o.If(storage.Conditions{DoesNotExist: true})
		// If the live object already exists in your bucket, set instead a
		// generation-match precondition using the live object's generation number.
		// attrs, err := o.Attrs(ctx)
		// if err != nil {
		// 	return fmt.Errorf("object.Attrs: %v", err)
		// }
		// o = o.If(storage.Conditions{GenerationMatch: attrs.Generation})

		// Upload an object with storage.Writer.
		wc := o.NewWriter(ctx)
		if _, err = io.Copy(wc, f); err != nil {
			return fmt.Errorf("while copying data to bucket: %w", err)
		}
		if err := wc.Close(); err != nil {
			return fmt.Errorf("while closing bucket writer: %w", err)
		}
	}

	return nil
}

func getGCSClient(ctx context.Context, url string) (*storage.Client, error) {
	if url == defaultGCSURL || url == "" {
		return storage.NewClient(ctx)
	}
	os.Setenv("STORAGE_EMULATOR_HOST", url)
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	httpClient := &http.Client{Transport: tr}
	return storage.NewClient(
		ctx,
		option.WithHTTPClient(httpClient),
	)
}

func updateIndex() error {
	return nil
}
