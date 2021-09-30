// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package github

import (
	"fmt"
	"log"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

type prProcessor struct {
	issueRegexp   *regexp.Regexp
	ignoredLabels map[string]struct{}
}

func newPRProcessor(repoName string, ignoredLabels map[string]struct{}) *prProcessor {
	return &prProcessor{
		issueRegexp:   regexp.MustCompile(fmt.Sprintf(`https://github.com/%s/issues/(\d+)`, repoName)),
		ignoredLabels: ignoredLabels,
	}
}

func (prp *prProcessor) extractPullRequests(resp *apiResponse) []PullRequest {
	var prs []PullRequest

MAIN_LOOP:
	for _, r := range resp.Data.Search.Nodes {
		if !r.Merged {
			continue
		}

		pr := PullRequest{
			Title:  r.Title,
			Number: r.Number,
		}

		if r.Labels != nil {
			pr.Labels = make(map[string]struct{}, len(r.Labels.Edges))

			for _, lbl := range r.Labels.Edges {
				lblName := lbl.Node.Name

				if _, ok := prp.ignoredLabels[lblName]; ok {
					continue MAIN_LOOP
				}

				pr.Labels[lblName] = struct{}{}
			}
		}

		pr.Issues = prp.extractIssues(r.BodyHTML)

		prs = append(prs, pr)
	}

	return prs
}

func (prp *prProcessor) extractIssues(body string) []int {
	node, err := html.Parse(strings.NewReader(body))
	if err != nil {
		log.Fatalf("Failed to parse HTML: %v\n%s", err, body)
	}

	linkedIssues := prp.findLinkedIssues(node)
	if len(linkedIssues) == 0 {
		return nil
	}

	issueList := make([]int, 0, len(linkedIssues))
	for i := range linkedIssues {
		issueList = append(issueList, i)
	}

	sort.Ints(issueList)

	return issueList
}

func (prp *prProcessor) findLinkedIssues(node *html.Node) map[int]struct{} {
	linkedIssues := map[int]struct{}{}

	if node.Type == html.ElementNode && node.Data == "a" {
		if issue := prp.extractIssueNumber(node); issue > 0 {
			linkedIssues[issue] = struct{}{}
		}
	}

	for c := node.FirstChild; c != nil; c = c.NextSibling {
		issues := prp.findLinkedIssues(c)
		for i := range issues {
			linkedIssues[i] = struct{}{}
		}
	}

	return linkedIssues
}

func (prp *prProcessor) extractIssueNumber(node *html.Node) int {
	attrs := make(map[string]string, len(node.Attr))
	for _, attr := range node.Attr {
		attrs[attr.Key] = attr.Val
	}

	if class, ok := attrs["class"]; ok && strings.Contains(class, "issue-link") {
		href := attrs["href"]
		matches := prp.issueRegexp.FindStringSubmatch(href)

		if len(matches) == 2 {
			issueNum, err := strconv.Atoi(matches[1])
			if err != nil {
				log.Fatalf("Failed to convert %s to number: %v", matches[1], err)
			}

			return issueNum
		}
	}

	return -1
}
