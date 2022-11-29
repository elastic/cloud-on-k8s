package helm

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
)

func Release() error {
	// remove existing releases
	if err := removeExistinReleases(); err != nil {
		return err
	}
	// upload charts
	if err := uploadCharts(); err != nil {
		return err
	}
	// update index
	if err := updateIndex(); err != nil {
		return err
	}
	return nil
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

func uploadCharts() error {
	return nil
}

func updateIndex() error {
	return nil
}
