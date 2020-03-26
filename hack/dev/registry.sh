#! /usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License;
# you may not use this file except in compliance with the Elastic License.

set -eu

# shellcheck disable=SC2016
help() { 
  echo \
'Manage a Docker registry in k8s.

Usage:
  registry.sh [command]

Commands:
    create              Deploy the registry k8s resources
    port-forward start  Start to forward the localhost port 5000 to the registry port 5000
    port-forward stop   Stop the port forwarding
    delete              Delete the registry k8s resources
    start               Execute the `create` and `port-forward start` commands'
}

kubectl-in-docker() {
  local build_tools_image=docker.elastic.co/k8s/build-tools

  # Build the image if it does not exist
  if [[ "$(docker images -q $build_tools_image 2> /dev/null)" == "" ]]; then
    local dockerfile_path=hack/dev/kubectl-image
    docker build -t $build_tools_image -f $dockerfile_path/Dockerfile $dockerfile_path
  fi

  # Remove potential existing container
  if [[ "$(docker ps -a --filter=name=registry-port-forwarder -q)" != "" ]]; then
    docker rm --force registry-port-forwarder
  fi

  docker run -d --name registry-port-forwarder --net=host \
    -v ~/.kube:/root/.kube -v ~/.minikube:"$HOME"/.minikube \
    $build_tools_image kubectl "$@"
}

main() {
  case $@ in
    start)
      main create
      main port-forward start
    ;;
    create)
      kubectl apply -f config/dev/registry.yaml
    ;;
    "port-forward start")
      kubectl wait pods -l k8s-app=kube-registry -n=kube-system --for condition=Ready --timeout 40s
      local podName
      podName=$(kubectl get po -n kube-system | grep kube-registry-v0 | awk '{print $1}')
      kubectl-in-docker port-forward -n=kube-system "$podName" 5000:5000
      docker exec registry-port-forwarder timeout 15 sh -c 'until nc -z localhost 5000; do sleep 0.5; done'
    ;;
    "port-forward stop")
      # Delete the container if it exists
      if [[ "$(docker ps -a --filter=name=registry-port-forwarder -q)" != "" ]]; then
        docker rm --force registry-port-forwarder
      fi
    ;;
    delete)
      kubectl delete -f config/dev/registry.yaml
    ;;
    *)
      help; exit 1
    ;;
  esac
}

main "$@"
