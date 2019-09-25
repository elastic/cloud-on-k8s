// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package detector

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/karrick/godirwalk"
)

var errLicenceNotFound = errors.New("failed to detect licence")

type Dependencies struct {
	Direct   []LicenceInfo
	Indirect []LicenceInfo
}

type LicenceInfo struct {
	Module
	LicenceFile string
	Error       error
}

type Module struct {
	Path     string     // module path
	Version  string     // module version
	Main     bool       // is this the main module?
	Time     *time.Time // time version was created
	Indirect bool       // is this module only an indirect dependency of main module?
	Dir      string     // directory holding files for this module, if any
	Replace  *Module    // replace directive
}

func Detect(data io.Reader, includeIndirect bool) (*Dependencies, error) {
	dependencies, err := parseDependencies(data, includeIndirect)
	if err != nil {
		return nil, err
	}

	err = detectLicences(dependencies)
	return dependencies, err
}

func parseDependencies(data io.Reader, includeIndirect bool) (*Dependencies, error) {
	deps := &Dependencies{}
	decoder := json.NewDecoder(data)
	for {
		var mod Module
		if err := decoder.Decode(&mod); err != nil {
			if err == io.EOF {
				return deps, nil
			}
			return deps, fmt.Errorf("failed to parse dependencies: %w", err)
		}

		if !mod.Main && mod.Dir != "" {
			if mod.Indirect {
				if includeIndirect {
					deps.Indirect = append(deps.Indirect, LicenceInfo{Module: mod})
				}
				continue
			}
			deps.Direct = append(deps.Direct, LicenceInfo{Module: mod})
		}
	}
}

func detectLicences(deps *Dependencies) error {
	licenceRegex := buildLicenceRegex()
	for _, depList := range [][]LicenceInfo{deps.Direct, deps.Indirect} {
		for i, dep := range depList {
			srcDir := dep.Dir
			if dep.Replace != nil {
				srcDir = dep.Replace.Dir
			}

			depList[i].LicenceFile, depList[i].Error = findLicenceFile(srcDir, licenceRegex)
			if depList[i].Error != nil && depList[i].Error != errLicenceNotFound {
				return fmt.Errorf("unexpected error while finding licence for %s in %s: %w", dep.Path, srcDir, depList[i].Error)
			}
		}
	}

	return nil
}

func buildLicenceRegex() *regexp.Regexp {
	// inspired by https://github.com/src-d/go-license-detector/blob/7961dd6009019bc12778175ef7f074ede24bd128/licensedb/internal/investigation.go#L29
	licenceFileNames := []string{
		`li[cs]en[cs]es?`,
		`legal`,
		`copy(left|right|ing)`,
		`unlicense`,
		`l?gpl([-_ v]?)(\d\.?\d)?`,
		`bsd`,
		`mit`,
		`apache`,
	}

	regexStr := fmt.Sprintf(`^(?i:(%s)(\.(txt|md|rst))?)$`, strings.Join(licenceFileNames, "|"))
	return regexp.MustCompile(regexStr)
}

func findLicenceFile(root string, licenceRegex *regexp.Regexp) (string, error) {
	errStopWalk := errors.New("stop walk")
	var licenceFile string
	err := godirwalk.Walk(root, &godirwalk.Options{
		Callback: func(osPathName string, dirent *godirwalk.Dirent) error {
			if licenceRegex.MatchString(dirent.Name()) {
				if dirent.IsDir() {
					return filepath.SkipDir
				}
				licenceFile = osPathName
				return errStopWalk
			}
			return nil
		},
		Unsorted: true,
	})

	if err != nil {
		if errors.Is(err, errStopWalk) {
			return licenceFile, nil
		}
		return "", err
	}

	return "", errLicenceNotFound
}
