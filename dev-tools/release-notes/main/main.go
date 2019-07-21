// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	baseURL             = "https://api.github.com/repos/"
	repo                = "elastic/cloud-on-k8s/"
	releaseNoteTemplate = `:issue: https://github.com/{{.Repo}}issues/
:pull: https://github.com/{{.Repo}}pull/

[[release-notes-{{.Version}}]]
== {n} version {{.Version}}
{{range $group, $prs := .Groups}}
[[{{- id $group -}}-{{$.Version}}]]
[float]
=== {{index $.GroupLabels $group}}
{{range $prs}}
* {{.Title}} {pull}{{.Number}}[#{{.Number}}]{{with .RelatedIssues -}}
{{$length := len .}} (issue{{if gt $length 1}}s{{end}}: {{range $idx, $el := .}}{{if $idx}}, {{end}}{issue}{{$el}}[#{{$el}}]{{end}})
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
		"nogroup":      "Misc",
	}

	ignore = map[string]bool{
		">non-issue":   true,
		">refactoring": true,
		">docs":        true,
		">test":        true,
		":ci":          true,
		"backport":     true,
	}
)

// Label models a subset of a GitHub label.
type Label struct {
	Name string `json:"name"`
}

// Issue models a subset of a Github issue.
type Issue struct {
	Labels        []Label           `json:"labels"`
	Body          string            `json:"body"`
	Title         string            `json:"title"`
	Number        int               `json:"number"`
	PullRequest   map[string]string `json:"pull_request,omitempty"`
	RelatedIssues []int
}

type GroupedIssues = map[string][]Issue

type TemplateParams struct {
	Version     string
	Repo        string
	GroupLabels map[string]string
	Groups      GroupedIssues
}

func fetch(url string, out interface{}) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}

	nextLink := extractNextLink(resp.Header)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", errors.New(fmt.Sprintf("%s: %d %s ", url, resp.StatusCode, resp.Status))
	}

	defer resp.Body.Close()
	if err = json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return nextLink, nil

}

func extractNextLink(headers http.Header) string {
	var nextLink string
	nextRe := regexp.MustCompile(`<([^>]+)>; rel="next"`)
	links := headers["Link"]
	for _, lnk := range links {
		matches := nextRe.FindAllStringSubmatch(lnk, 1)
		if matches != nil && matches[0][1] != "" {
			nextLink = matches[0][1]
			break
		}
	}
	return nextLink
}

func fetchVersionLabels() ([]string, error) {
	var versionLabels []string
	url := fmt.Sprintf("%s%slabels?page=1", baseURL, repo)
FETCH:
	var labels []Label
	next, err := fetch(url, &labels)
	if err != nil {
		return nil, err
	}
	for _, l := range labels {
		if strings.HasPrefix(l.Name, "v") {
			versionLabels = append(versionLabels, l.Name)
		}
	}
	if next != "" {
		url = next
		goto FETCH
	}

	return versionLabels, nil
}

func fetchIssues(version string) (GroupedIssues, error) {
	url := fmt.Sprintf("%s%sissues?labels=%s&pagesize=100&state=all&page=1", baseURL, repo, version)
	var prs []Issue
FETCH:
	var tranche []Issue
	next, err := fetch(url, &tranche)
	if err != nil {
		return nil, err
	}
	for _, issue := range tranche {
		// only look at PRs
		if issue.PullRequest != nil {
			prs = append(prs, issue)
		}
	}
	if next != "" {
		url = next
		goto FETCH
	}
	result := make(GroupedIssues)
	noGroup := "nogroup"
PR:
	for _, pr := range prs {
		prLabels := make(map[string]bool)
		for _, lbl := range pr.Labels {
			// remove PRs that have labels to be ignored
			if ignore[lbl.Name] {
				continue PR
			}
			// build a lookup table of all labels for this PR
			prLabels[lbl.Name] = true
		}

		// extract related issues from PR body
		if err := extractRelatedIssues(&pr); err != nil {
			return nil, err
		}

		// group PRs by type label
		for typeLabel := range groupLabels {
			if prLabels[typeLabel] {
				result[typeLabel] = append(result[typeLabel], pr)
				continue PR
			}
		}
		// or fall back to a default group
		result[noGroup] = append(result[noGroup], pr)
	}
	return result, nil
}

func extractRelatedIssues(issue *Issue) error {
	re := regexp.MustCompile(fmt.Sprintf(`https://github.com/%sissues/(\d+)`, repo))
	matches := re.FindAllStringSubmatch(issue.Body, -1)
	issues := map[int]struct{}{}
	for _, capture := range matches {
		issueNum, err := strconv.Atoi(capture[1])
		if err != nil {
			return err
		}
		issues[issueNum] = struct{}{}

	}
	for rel := range issues {
		issue.RelatedIssues = append(issue.RelatedIssues, rel)
	}
	sort.Ints(issue.RelatedIssues)
	return nil
}

func dumpIssues(params TemplateParams, out io.Writer) {
	funcs := template.FuncMap{
		"id": func(s string) string {
			return strings.TrimPrefix(s, ">")
		},
	}
	tpl := template.Must(template.New("release_notes").Funcs(funcs).Parse(releaseNoteTemplate))
	err := tpl.Execute(out, params)
	if err != nil {
		println(err)
	}
}

func main() {
	labels, err := fetchVersionLabels()
	if err != nil {
		panic(err)
	}

	if len(os.Args) != 2 {
		usage(labels)
	}

	version := os.Args[1]
	found := false
	for _, l := range labels {
		if l == version {
			found = true
		}
	}
	if !found {
		usage(labels)
	}

	groupedIssues, err := fetchIssues(version)
	if err != nil {
		panic(err)
	}
	dumpIssues(TemplateParams{
		Version:     strings.TrimPrefix(version, "v"),
		Repo:        repo,
		GroupLabels: groupLabels,
		Groups:      groupedIssues,
	}, os.Stdout)

}

func usage(labels []string) {
	println(fmt.Sprintf("USAGE: %s version > outfile", os.Args[0]))
	println("Known versions:")
	sort.Strings(labels)
	for _, l := range labels {
		println(l)
	}
	os.Exit(1)
}
