// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package detector

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/elastic/cloud-on-k8s/hack/licence-detector/dependency"
	"github.com/google/licenseclassifier"
	"github.com/karrick/godirwalk"
)

// detectionThreshold is the minimum confidence score required from the licence classifier.
const detectionThreshold = 0.85

var errLicenceNotFound = errors.New("failed to detect licence")

type dependencies struct {
	direct   []*module
	indirect []*module
}

type module struct {
	Path     string     // module path
	Version  string     // module version
	Main     bool       // is this the main module?
	Time     *time.Time // time version was created
	Indirect bool       // is this module only an indirect dependency of main module?
	Dir      string     // directory holding files for this module, if any
	Replace  *module    // replace directive
}

// NewClassifier creates a new instance of the licence classifier.
func NewClassifier(dataPath string) (*licenseclassifier.License, error) {
	absPath, err := filepath.Abs(dataPath)
	if err != nil {
		return nil, fmt.Errorf("failed to determine absolute path of licence data file: %w", err)
	}

	return licenseclassifier.New(detectionThreshold, licenseclassifier.Archive(absPath))
}

// Detect searches the dependencies on disk and detects licences.
func Detect(data io.Reader, classifier *licenseclassifier.License, overrides dependency.Overrides, includeIndirect bool) (*dependency.List, error) {
	// parse the output of go mod list
	deps, err := parseDependencies(data, includeIndirect)
	if err != nil {
		return nil, err
	}

	// find licences for each dependency
	return detectLicences(classifier, deps, overrides)
}

func parseDependencies(data io.Reader, includeIndirect bool) (*dependencies, error) {
	deps := &dependencies{}
	decoder := json.NewDecoder(data)
	for {
		var mod module
		if err := decoder.Decode(&mod); err != nil {
			if errors.Is(err, io.EOF) {
				return deps, nil
			}
			return deps, fmt.Errorf("failed to parse dependencies: %w", err)
		}

		if !mod.Main && mod.Dir != "" {
			if mod.Indirect {
				if includeIndirect {
					deps.indirect = append(deps.indirect, &mod)
				}
				continue
			}
			deps.direct = append(deps.direct, &mod)
		}
	}
}

func detectLicences(classifier *licenseclassifier.License, deps *dependencies, overrides dependency.Overrides) (*dependency.List, error) {
	depList := &dependency.List{}
	licenceRegex := buildLicenceRegex()

	var err error
	if depList.Direct, err = doDetectLicences(licenceRegex, classifier, deps.direct, overrides); err != nil {
		return depList, err
	}

	if depList.Indirect, err = doDetectLicences(licenceRegex, classifier, deps.indirect, overrides); err != nil {
		return depList, err
	}

	return depList, nil
}

func doDetectLicences(licenceRegex *regexp.Regexp, classifier *licenseclassifier.License, depList []*module, overrides dependency.Overrides) ([]dependency.Info, error) {
	if len(depList) == 0 {
		return nil, nil
	}

	// this is not an exhaustive list of Elastic-approved licences, but includes all the ones we use to date
	whitelist := map[string]struct{}{
		"Apache-2.0":   struct{}{},
		"BSD-2-Clause": struct{}{},
		"BSD-3-Clause": struct{}{},
		"ISC":          struct{}{},
		"MIT":          struct{}{},
		// Yellow list: Mozilla Public License 1.1 or 2.0 (“MPL”) Exception:
		// "Incorporation of unmodified source or binaries into Elastic products is permitted,
		// provided that the product's NOTICE file links to a URL providing the MPL-covered source code"
		// We do not modify any of the dependencies and we link to the source code, so we are okay.
		"MPL-2.0":       struct{}{},
		"Public Domain": struct{}{},
	}

	depInfoList := make([]dependency.Info, len(depList))
	for i, mod := range depList {
		depInfo := mkDepInfo(mod, overrides)

		// find the licence file if the override hasn't provided one
		if depInfo.LicenceFile == "" {
			var err error
			depInfo.LicenceFile, err = findLicenceFile(depInfo.Dir, licenceRegex)
			if err != nil && !errors.Is(err, errLicenceNotFound) {
				return nil, fmt.Errorf("failed to find licence file for %s in %s: %w", depInfo.Name, depInfo.Dir, err)
			}
		}

		// detect the licence type if the override hasn't provided one
		if depInfo.LicenceType == "" {
			if depInfo.LicenceFile == "" {
				return nil, fmt.Errorf("no licence file found for %s. Add an override entry with licence type to continue.", depInfo.Name)
			}

			var err error
			depInfo.LicenceType, err = detectLicenceType(classifier, depInfo.LicenceFile)
			if err != nil {
				return nil, fmt.Errorf("failed to detect licence type of %s from %s: %w", depInfo.Name, depInfo.LicenceFile, err)
			}

			if depInfo.LicenceType == "" {
				return nil, fmt.Errorf("licence unknown for %s. Add an override entry with licence type to continue.", depInfo.Name)
			}
		}

		if _, ok := whitelist[depInfo.LicenceType]; !ok {
			return nil, fmt.Errorf("dependency %s uses licence %s which is not whitelisted", depInfo.Name, depInfo.LicenceType)
		}
		depInfoList[i] = depInfo
	}

	return depInfoList, nil
}

func mkDepInfo(mod *module, overrides dependency.Overrides) dependency.Info {
	m := mod
	if mod.Replace != nil {
		m = mod.Replace
	}

	override, ok := overrides[m.Path]
	if !ok {
		override = dependency.Info{}
	}

	return dependency.Info{
		Name:                    m.Path,
		Dir:                     coalesce(override.Dir, m.Dir),
		Version:                 coalesce(override.Version, m.Version),
		VersionTime:             coalesce(override.VersionTime, m.Time.Format(time.RFC3339)),
		URL:                     determineURL(override.URL, m.Path),
		LicenceFile:             override.LicenceFile,
		LicenceType:             override.LicenceType,
		LicenceTextOverrideFile: override.LicenceTextOverrideFile,
	}
}

func coalesce(a, b string) string {
	if a != "" {
		return a
	}

	return b
}

func determineURL(overrideURL, modulePath string) string {
	if overrideURL != "" {
		return overrideURL
	}

	parts := strings.Split(modulePath, "/")
	switch parts[0] {
	case "github.com":
		// GitHub URLs that have more than two path elements will return a 404 (e.g. https://github.com/elazarl/goproxy/ext).
		// We strip out the extra path elements from the end to come up with a valid URL like https://github.com/elazarl/goproxy/.
		if len(parts) > 3 {
			return "https://" + strings.Join(parts[:3], "/")
		}
		return "https://" + modulePath
	case "k8s.io":
		return "https://github.com/kubernetes/" + parts[1]
	default:
		return "https://" + modulePath
	}
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
		Unsorted: false,
	})

	if err != nil {
		if errors.Is(err, errStopWalk) {
			return licenceFile, nil
		}
		return "", err
	}

	return "", errLicenceNotFound
}

func detectLicenceType(classifier *licenseclassifier.License, licenceFile string) (string, error) {
	contents, err := ioutil.ReadFile(licenceFile)
	if err != nil {
		return "", fmt.Errorf("failed to read licence content from %s: %w", licenceFile, err)
	}

	matches := classifier.MultipleMatch(string(contents), true)
	// there should be at least one match
	if len(matches) < 1 {
		return "", fmt.Errorf("failed to detect licence type of %s", licenceFile)
	}

	// matches are sorted by confidence such that the first result has the highest confidence level
	return matches[0].Name, nil
}
