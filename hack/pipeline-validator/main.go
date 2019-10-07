// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// This tool is used to validate Jenkins pipelines.
// Based on https://jenkins.io/doc/book/pipeline/development/#linter

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const JenkinsURL string = "https://devops-ci.elastic.co"

var client = http.Client{Timeout: 60 * time.Second}

type CSRFToken struct {
	Crumb             string `json:"crumb"`
	CrumbRequestField string `json:"crumbRequestField"`
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	fmt.Println("Starting validation of Jenkins pipelines...")

	pipelines, err := listPipelines()
	if err != nil {
		log.Fatalln("Fail to list pipelines, err:", err)
	}

	fmt.Println("Getting Jenkins CSRF token...")
	token, err := getToken()
	if err != nil {
		log.Fatalln("Fail to retrieve Jenkins CSRF token, err:", err)
	}

	for _, p := range pipelines {
		_, err := validate(token, v)
		fmt.Println("Validating", v)
		if err != nil {
			log.Println("Fail to validate", v)
			log.Fatalln(err)
		}
	}

	fmt.Println("Validation was successful!")
}

func listPipelines() ([]string, error) {
	var files []string
	err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if !info.IsDir() {
			if strings.Contains(info.Name(), "Jenkinsfile") || strings.Contains(info.Name(), ".jenkinsfile") {
				files = append(files, path)
			}
		}
		return nil
	})
	return files, err
}

func getToken() (*CSRFToken, error) {
	req, err := http.NewRequest("GET", JenkinsURL+"/crumbIssuer/api/json", http.NoBody)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, errors.New(string(resp.StatusCode))
	}

	bytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	tokenJSON := &CSRFToken{}
	err = json.Unmarshal(bytes, tokenJSON)
	if err != nil {
		return nil, err
	}

	return tokenJSON, nil
}

func validate(token *CSRFToken, pipeline string) (string, error) {
	bytez, err := ioutil.ReadFile(pipeline)
	if err != nil {
		return "", err
	}

	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	err = w.WriteField("jenkinsfile", string(bytez))
	if err != nil {
		return "", err
	}
	w.Close()

	req, err := http.NewRequest("POST", JenkinsURL+"/pipeline-model-converter/validate", &b)
	if err != nil {
		return "", err
	}
	req.Header.Add(token.CrumbRequestField, token.Crumb)
	req.Header.Add("Content-Type", w.FormDataContentType())

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", errors.New(string(resp.StatusCode))
	}

	bytez, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	jenkinsResponse := string(bytez)
	if !strings.Contains(jenkinsResponse, "Jenkinsfile successfully validated") {
		return "", errors.New(jenkinsResponse)
	}

	return jenkinsResponse, nil
}
