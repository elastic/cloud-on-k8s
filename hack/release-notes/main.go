// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"text/template"

	"github.com/elastic/cloud-on-k8s/v3/hack/release-notes/github"
)

const (
	noGroup  = "nogroup"
	repoName = "elastic/cloud-on-k8s"

	releaseNotesTemplate = `## {{$.Version}} [elastic-cloud-kubernetes-{{replace $.Version "." "" }}-release-notes]

{{range $group := .GroupOrder -}}
{{$grouplbl := index $.LabelMapping $group}}
{{with (index $.Groups $grouplbl)}}
{{$header := index $.GroupLabels $grouplbl}}
### {{$header}}  [elastic-cloud-kubernetes-{{replace $.Version "." ""}}-{{replace $header " " "-"}}]
{{range .}}
- {{.Title}} [#{{.Number}}](https://github.com/{{$.Repo}}/pull/{{.Number}}){{with .Issues -}}
{{$length := len .}} (issue{{if gt $length 1}}s{{end}}: {{range $idx, $el := .}}{{if $idx}}, {{end}}[#{{$el}}](https://github.com/{{$.Repo}}/issues/{{$el}}){{end}})
{{- end}}
{{- end}}
{{- end}}
{{end}}
### Miscellaneous  [elastic-cloud-kubernetes-{{$.Version}}-miscellaneous]

:::{dropdown} Updated dependencies
{{range $updatedDep := .UpdatedDeps -}}
- {{$updatedDep}}
{{end -}}:::
`
)

var (
	groupLabels = map[string]string{
		"breaking":    "Breaking changes",
		"deprecation": "Deprecations",
		"feature":     "Features and enhancements",
		"bug":         "Fixes",
		"docs":        "Documentation improvements",
		noGroup:       "Miscellaneous",
	}

	groupOrder = []string{
		">breaking",
		">deprecation",
		">feature", // This will include both >feature and >enhancement
		">bug",
		">docs",
		noGroup,
	}

	labelMapping = map[string]string{
		">bug":         "bug",
		">docs":        "docs",
		">feature":     "feature",
		">enhancement": "feature",
		">deprecation": "deprecation",
		">breaking":    "breaking",
		noGroup:        noGroup,
	}

	ignoredLabels = map[string]struct{}{
		">non-issue":                 {},
		">refactoring":               {},
		">test":                      {},
		">renovate":                  {},
		"dependencies":               {},
		":ci":                        {},
		"backport":                   {},
		"exclude-from-release-notes": {},
	}
)

func main() {
	if len(os.Args) != 2 && len(os.Args) != 3 {
		fmt.Printf("Usage: GH_TOKEN=<github token> %s VERSION [PREVIOUS_VERSION]\n", os.Args[0])
		os.Exit(2)
	}
	ctx := context.Background()

	version := os.Args[1]

	prs, err := github.LoadPullRequests(ctx, repoName, version, ignoredLabels)
	if err != nil {
		log.Printf("ERROR: %v", err)
		os.Exit(1)
	}

	if len(prs) == 0 {
		log.Print("No pull requests found. Check the version argument.")
		os.Exit(2)
	}

	groupedPRs := groupPullRequests(prs)

	var updatedDeps []string
	if len(os.Args) > 2 {
		previousVersion := os.Args[2]
		updatedDeps = github.GoDiff(ctx, repoName, previousVersion, version)
	} else {
		_, err := fmt.Fprintln(os.Stderr, "No previous version provided, skipping go.mod diff.")
		if err != nil {
			panic(err)
		}
	}

	if err := render(version, groupedPRs, updatedDeps); err != nil {
		log.Printf("Failed to render release notes: %v", err)
		os.Exit(1)
	}
}

func groupPullRequests(prs []github.PullRequest) map[string][]github.PullRequest {
	groups := make(map[string][]github.PullRequest)

PR_LOOP:
	for _, pr := range prs {
		for prLabel, mappedLabel := range labelMapping {
			if _, ok := pr.Labels[prLabel]; ok {
				groups[mappedLabel] = append(groups[mappedLabel], pr)
				continue PR_LOOP
			}
		}

		groups[noGroup] = append(groups[noGroup], pr)
	}

	return groups
}

func render(version string, groups map[string][]github.PullRequest, updatedDeps []string) error {
	params := struct {
		Version      string
		Repo         string
		Groups       map[string][]github.PullRequest
		GroupLabels  map[string]string
		GroupOrder   []string
		LabelMapping map[string]string
		UpdatedDeps  []string
	}{
		Version:      version,
		Repo:         repoName,
		Groups:       groups,
		GroupLabels:  groupLabels,
		GroupOrder:   groupOrder,
		LabelMapping: labelMapping,
		UpdatedDeps:  updatedDeps,
	}

	funcs := template.FuncMap{
		"id": func(s string) string {
			return strings.TrimPrefix(s, ">")
		},
		"replace": func(input, old, newStr string) string {
			replacedString := strings.ReplaceAll(input, old, newStr)
			return strings.ToLower(replacedString)
		},
	}

	tpl := template.Must(template.New("release_notes").Funcs(funcs).Parse(releaseNotesTemplate))

	return tpl.Execute(os.Stdout, params)
}
