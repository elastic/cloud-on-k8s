// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package main

import (
	"bytes"
	"net/http"
	"reflect"
	"testing"
)

func Test_dumpIssues(t *testing.T) {
	type args struct {
		params TemplateParams
	}
	tests := []struct {
		name    string
		args    args
		wantOut string
	}{
		{
			name: "two issues--no related",
			args: args{
				params: TemplateParams{
					Version: "0.9.0",
					Repo:    "me/my-repo/",
					GroupLabels: map[string]string{
						">bugs": "Bug Fixes",
					},
					Groups: GroupedIssues{
						">bugs": []Issue{
							{
								Labels:        nil,
								Body:          "body",
								Title:         "title",
								Number:        123,
								PullRequest:   nil,
								RelatedIssues: nil,
							},
							{
								Labels:        nil,
								Body:          "body2",
								Title:         "title2",
								Number:        456,
								PullRequest:   nil,
								RelatedIssues: nil,
							},
						},
					},
				},
			},
			wantOut: `:issue: https://github.com/me/my-repo/issues/
:pull: https://github.com/me/my-repo/pull/

[[release-notes-0.9.0]]
== {n} version 0.9.0

[[bugs-0.9.0]]
[float]
=== Bug Fixes

* title {pull}123[#123]
* title2 {pull}456[#456]

`,
		},
		{
			name: "single issue with related",
			args: args{
				params: TemplateParams{
					Version: "0.9.0",
					Repo:    "me/my-repo/",
					GroupLabels: map[string]string{
						">bugs": "Bug Fixes",
					},
					Groups: GroupedIssues{
						">bugs": []Issue{
							{
								Labels:        nil,
								Body:          "body",
								Title:         "title",
								Number:        123,
								PullRequest:   nil,
								RelatedIssues: []int{456},
							},
						},
					},
				},
			},
			wantOut: `:issue: https://github.com/me/my-repo/issues/
:pull: https://github.com/me/my-repo/pull/

[[release-notes-0.9.0]]
== {n} version 0.9.0

[[bugs-0.9.0]]
[float]
=== Bug Fixes

* title {pull}123[#123] (issue: {issue}456[#456])

`,
		},
		{
			name: "single issue--two related",
			args: args{
				params: TemplateParams{
					Version: "0.9.0",
					Repo:    "me/my-repo/",
					GroupLabels: map[string]string{
						">bugs": "Bug Fixes",
					},
					Groups: GroupedIssues{
						">bugs": []Issue{
							{
								Labels:        nil,
								Body:          "body",
								Title:         "title",
								Number:        123,
								PullRequest:   nil,
								RelatedIssues: []int{456, 789},
							},
						},
					},
				},
			},
			wantOut: `:issue: https://github.com/me/my-repo/issues/
:pull: https://github.com/me/my-repo/pull/

[[release-notes-0.9.0]]
== {n} version 0.9.0

[[bugs-0.9.0]]
[float]
=== Bug Fixes

* title {pull}123[#123] (issues: {issue}456[#456], {issue}789[#789])

`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := &bytes.Buffer{}
			dumpIssues(tt.args.params, out)
			if gotOut := out.String(); gotOut != tt.wantOut {
				t.Errorf("dumpIssues() = %v, want %v", gotOut, tt.wantOut)
			}
		})
	}
}

func Test_extractRelatedIssues(t *testing.T) {
	type args struct {
		issue *Issue
	}
	tests := []struct {
		name    string
		args    args
		want    []int
		wantErr bool
	}{
		{
			name: "single issue",
			args: args{
				issue: &Issue{
					Body: "Resolves https://github.com/elastic/cloud-on-k8s/issues/1241\r\n\r\n* If there is no existing annotation on a resource",
				},
			},
			want:    []int{1241},
			wantErr: false,
		},
		{
			name: "multi issue",
			args: args{
				issue: &Issue{
					Body: "Resolves https://github.com/elastic/cloud-on-k8s/issues/1241\r\n\r\nRelated https://github.com/elastic/cloud-on-k8s/issues/1245\r\n\r\n",
				},
			},
			want:    []int{1241, 1245},
			wantErr: false,
		},
		{
			name: "non issue",
			args: args{
				issue: &Issue{
					Body: "Resolves https://github.com/elastic/cloud-on-k8s/issues/1241\r\n\r\nSee all issues https://github.com/elastic/cloud-on-k8s/issues/\r\n\r\n",
				},
			},
			want:    []int{1241},
			wantErr: false,
		},
		{
			name: "duplicate issue",
			args: args{
				issue: &Issue{
					Body: "Resolves https://github.com/elastic/cloud-on-k8s/issues/1241\r\n\r\nRelated https://github.com/elastic/cloud-on-k8s/issues/1241\r\n\r\n",
				},
			},
			want:    []int{1241},
			wantErr: false,
		},
		{
			name: "ordered",
			args: args{
				issue: &Issue{
					Body: "Resolves https://github.com/elastic/cloud-on-k8s/issues/1245\r\n\r\nRelated https://github.com/elastic/cloud-on-k8s/issues/1241\r\n\r\n",
				},
			},
			want:    []int{1241, 1245},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := extractRelatedIssues(tt.args.issue); (err != nil) != tt.wantErr {
				t.Errorf("extractRelatedIssues() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(tt.want, tt.args.issue.RelatedIssues) {
				t.Errorf("extractRelatedIssues() got = %v, want %v", tt.args.issue.RelatedIssues, tt.want)
			}
		})
	}
}

func Test_extractNextLink(t *testing.T) {
	testFixture := "https://api.github.com/repositories/155368246/issues?page=2"
	type args struct {
		headers http.Header
	}
	tests := []struct {
		name string
		args args
		want *string
	}{
		{
			name: "no link",
			args: args{
				headers: http.Header{},
			},
			want: nil,
		},
		{
			name: "with next link",
			args: args{
				headers: http.Header{
					"Link": []string{
						`<https://api.github.com/repositories/155368246/issues?page=2>; rel="next", <https://api.github.com/repositories/155368246/issues?page=6>; rel="last"`,
					},
				},
			},
			want: &testFixture,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractNextLink(tt.args.headers); got != tt.want {
				if got != nil && tt.want != nil {
					if *got != *tt.want {
						t.Errorf("extractNextLink() = %v, want %v", *got, *tt.want)
					}
				} else {
					t.Errorf("extractNextLink() = %v, want %v", got, tt.want)
				}

			}
		})
	}
}
