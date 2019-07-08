// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package main

import (
	"fmt"
	"log"
	"os"

	"github.com/heroku/docker-registry-client/registry"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	hub, err := registry.New("https://docker.elastic.co/", "", "")
	if err != nil {
		log.Println("Can't connect to a Docker registry...")
		log.Fatal(err)
	}

	tags, err := hub.Tags("eck/eck-operator")
	if err != nil {
		log.Println("Can't get a list of tags for Docker image...")
		log.Fatal(err)
	}

	tag := os.Getenv("TAG_NAME")
	if tag == "" {
		log.Fatal("Can't get tag from env variable...")
	}

	fmt.Println("Searching for a tag:", tag)
	fmt.Println("in the list of tags:")
	for _, v := range tags {
		fmt.Println(v)
	}

	if inTheList(tags, tag) {
		log.Fatal("Image already existed. No need to run release job.")
	}
}

func inTheList(list []string, entry string) bool {
	for _, v := range list {
		if v == entry {
			return true
		}
	}
	return false
}
