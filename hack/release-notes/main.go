// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"text/template"

	"github.com/elastic/cloud-on-k8s/hack/release-notes/github"
)

const (
	noGroup  = "nogroup"
	repoName = "elastic/cloud-on-k8s"

	releaseNotesTemplate = `:issue: https://github.com/{{.Repo}}/issues/
:pull: https://github.com/{{.Repo}}/pull/

[[release-notes-{{.Version}}]]
== {n} version {{.Version}}
{{range $group := .GroupOrder -}}
{{with (index $.Groups $group)}}
[[{{- id $group -}}-{{$.Version}}]]
[float]
=== {{index $.GroupLabels $group}}
{{range .}}
* {{.Title}} {pull}{{.Number}}[#{{.Number}}]{{with .Issues -}}
{{$length := len .}} (issue{{if gt $length 1}}s{{end}}: {{range $idx, $el := .}}{{if $idx}}, {{end}}{issue}{{$el}}[#{{$el}}]{{end}})
{{- end}}
{{- end}}
{{- end}}
{{end}}
`
)

var (
	groupLabels = map[string]string{
		">breaking":    "Breaking changes",
		">deprecation": "Deprecations",
		">feature":     "New features",
		">enhancement": "Enhancements",
		">bug":         "Bug fixes",
		noGroup:        "Misc",
	}

	groupOrder = []string{
		">breaking",
		">deprecation",
		">feature",
		">enhancement",
		">bug",
		noGroup,
	}

	ignoredLabels = map[string]struct{}{
		">non-issue":                 {},
		">refactoring":               {},
		">docs":                      {},
		">test":                      {},
		":ci":                        {},
		"backport":                   {},
		"exclude-from-release-notes": {},
	}
)

func main() {
	if len(os.Args) != 2 {
		fmt.Printf("Usage: GH_TOKEN=<github token> %s VERSION\n", os.Args[0])
		os.Exit(2)
	}

	version := os.Args[1]

	prs, err := github.LoadPullRequests(repoName, version, ignoredLabels)
	if err != nil {
		log.Printf("ERROR: %v", err)
		os.Exit(1)
	}

	if len(prs) == 0 {
		log.Print("No pull requests found. Check the version argument.")
		os.Exit(2)
	}

	groupedPRs := groupPullRequests(prs)
	if err := render(version, groupedPRs); err != nil {
		log.Printf("Failed to render release notes: %v", err)
		os.Exit(1)
	}
}

func groupPullRequests(prs []github.PullRequest) map[string][]github.PullRequest {
	groups := make(map[string][]github.PullRequest)

PR_LOOP:
	for _, pr := range prs {
		for _, lbl := range groupOrder {
			if _, ok := pr.Labels[lbl]; ok {
				groups[lbl] = append(groups[lbl], pr)
				continue PR_LOOP
			}
		}

		groups[noGroup] = append(groups[noGroup], pr)
	}

	return groups
}

func render(version string, groups map[string][]github.PullRequest) error {
	params := struct {
		Version     string
		Repo        string
		Groups      map[string][]github.PullRequest
		GroupLabels map[string]string
		GroupOrder  []string
	}{
		Version:     version,
		Repo:        repoName,
		Groups:      groups,
		GroupLabels: groupLabels,
		GroupOrder:  groupOrder,
	}

	funcs := template.FuncMap{
		"id": func(s string) string {
			return strings.TrimPrefix(s, ">")
		},
	}

	tpl := template.Must(template.New("release_notes").Funcs(funcs).Parse(releaseNotesTemplate))

	return tpl.Execute(os.Stdout, params)
}
