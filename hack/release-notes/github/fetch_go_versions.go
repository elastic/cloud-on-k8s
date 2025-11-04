// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package github

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
)

// fetchFile fetches the content of a file from a GitHub repository at a specific branch.
func fetchFile(repoName, branch, path string) (string, error) {
	client := mkClient()
	url := fmt.Sprintf("https://api.github.com/repos/%s/contents/%s?ref=%s", repoName, path, branch)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github.v3.raw")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("failed to fetch %s: %s", url, resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

// parseDirectDeps extracts only top-level "require" entries (not indirect ones)
func parseDirectDeps(content string) map[string]string {
	deps := make(map[string]string)
	inRequire := false

	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if strings.HasPrefix(line, "require (") {
			inRequire = true
			continue
		}
		if inRequire && line == ")" {
			inRequire = false
			continue
		}

		if inRequire && !strings.HasPrefix(line, "//") && line != "" {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				mod := fields[0]
				ver := fields[1]
				if !strings.Contains(ver, "// indirect") {
					deps[mod] = ver
				}
			}
		}

		// handle single-line requires: require github.com/foo/bar v1.0.0
		if strings.HasPrefix(line, "require ") && !strings.Contains(line, "(") {
			fields := strings.Fields(line)
			if len(fields) >= 3 {
				mod := fields[1]
				ver := fields[2]
				if !strings.Contains(ver, "// indirect") {
					deps[mod] = ver
				}
			}
		}
	}
	return deps
}

const (
	goMod      = "go.mod"
	dockerFile = "build/Dockerfile"
)

var (
	GoVersion = regexp.MustCompile(`go:([0-9]+\.[0-9]+\.[0-9]+)`)
)

func GoDiff(repo, oldVersion, newVersion string) []string {
	// Get Go versions
	dockerFile, err := fetchFile(repo, "v"+newVersion, dockerFile)
	if err != nil {
		panic(err)
	}
	newGoVersion := GoVersionFromDockerfile(dockerFile)

	baseContent, err := fetchFile(repo, "v"+oldVersion, goMod)
	if err != nil {
		panic(err)
	}
	headContent, err := fetchFile(repo, "v"+newVersion, goMod)
	if err != nil {
		panic(err)
	}
	baseDeps := parseDirectDeps(baseContent)
	headDeps := parseDirectDeps(headContent)
	result := make([]string, 0, len(headDeps))
	result = append(result, fmt.Sprintf("Go %s", newGoVersion))
	for mod, newVer := range headDeps {
		oldVer := baseDeps[mod]
		if oldVer != "" && oldVer != newVer {
			result = append(result, fmt.Sprintf("%s %s", mod, newVer))
		}
	}
	sort.Strings(result)
	return result
}

func GoVersionFromDockerfile(dockerfileContent string) string {
	matches := GoVersion.FindStringSubmatch(dockerfileContent)
	if len(matches) == 2 {
		return matches[1]
	}
	return ""
}
