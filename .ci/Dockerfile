# Docker image used for continuous integration
FROM golang:1.13-stretch

# TODO: update to >1.24.0 once https://github.com/golangci/golangci-lint/issues/994 is fixed, as jenkins OOMs when running 1.24.0
ENV GOLANGCILINT_VERSION=1.23.8
ENV SHELLCHECK_VERSION=0.7.0
ENV KUBEBUILDER_VERSION=2.0.0
ENV GCLOUD_VERSION=277.0.0
ENV KUBECTL_VERSION=1.14.7
ENV DOCKER_VERSION=18.03.1-ce
ENV GOTESTSUM_VERSION=0.3.5
ENV KIND_VERSION=0.7.0
ENV OPENSHIFT_TOOLS_VERSION=4.3.0

# golangci-lint
RUN curl -sfL https://install.goreleaser.com/github.com/golangci/golangci-lint.sh \
    | sh -s -- -b $(go env GOPATH)/bin v${GOLANGCILINT_VERSION}

# kubebuilder to get required tools (etcd, apiserver, etc.)
ENV PATH=${PATH}:/usr/local/kubebuilder/bin
RUN curl -fsSLO https://github.com/kubernetes-sigs/kubebuilder/releases/download/v${KUBEBUILDER_VERSION}/kubebuilder_${KUBEBUILDER_VERSION}_linux_amd64.tar.gz && \
    tar -zxf kubebuilder_${KUBEBUILDER_VERSION}_linux_amd64.tar.gz && \
    mv kubebuilder_${KUBEBUILDER_VERSION}_linux_amd64 /usr/local/kubebuilder && \
    rm kubebuilder_${KUBEBUILDER_VERSION}_linux_amd64.tar.gz

# gcloud to provision GKE clusters
ENV PATH=${PATH}:/usr/local/google-cloud-sdk/bin
RUN curl -fsSLO https://dl.google.com/dl/cloudsdk/channels/rapid/downloads/google-cloud-sdk-${GCLOUD_VERSION}-linux-x86_64.tar.gz && \
    mkdir -p /usr/local/gcloud && \
    tar -zxf google-cloud-sdk-${GCLOUD_VERSION}-linux-x86_64.tar.gz -C /usr/local && \
    /usr/local/google-cloud-sdk/install.sh && \
    gcloud config set core/disable_usage_reporting true && \
    gcloud config set component_manager/disable_update_check true && \
    gcloud components install beta --quiet && \
    rm google-cloud-sdk-${GCLOUD_VERSION}-linux-x86_64.tar.gz

# kubectl for deploying the operator and running e2e tests
RUN curl -fsSLO https://storage.googleapis.com/kubernetes-release/release/v${KUBECTL_VERSION}/bin/linux/amd64/kubectl && \
    mv kubectl /usr/local/bin/kubectl && chmod +x /usr/local/bin/kubectl

# Docker client to build and push images
RUN curl -fsSLO https://download.docker.com/linux/static/stable/x86_64/docker-${DOCKER_VERSION}.tgz && \
    tar xzf docker-${DOCKER_VERSION}.tgz --strip 1 -C /usr/local/bin docker/docker && \
    rm docker-${DOCKER_VERSION}.tgz

# AWS CLI to upload YAML on S3 and xz-utils to decompress shellcheck
RUN apt-get update && apt-get --no-install-recommends -y install \
    awscli xz-utils && \
    apt-get clean && apt-get autoclean && \
    rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*

# shellcheck
ENV PATH=${PATH}:/usr/local/shellcheck
RUN curl -fsSLO https://github.com/koalaman/shellcheck/releases/download/v${SHELLCHECK_VERSION}/shellcheck-v${SHELLCHECK_VERSION}.linux.x86_64.tar.xz && \
    tar -xf shellcheck-v${SHELLCHECK_VERSION}.linux.x86_64.tar.xz && \
    mv shellcheck-v${SHELLCHECK_VERSION} /usr/local/shellcheck && \
    rm shellcheck-v${SHELLCHECK_VERSION}.linux.x86_64.tar.xz

# Azure CLI to provision AKS clusters
RUN curl -sSL https://aka.ms/InstallAzureCLIDeb | bash

# gotestsum to generate xUnit compatible XML for tests
RUN curl -fsSLO https://github.com/gotestyourself/gotestsum/releases/download/v${GOTESTSUM_VERSION}/gotestsum_${GOTESTSUM_VERSION}_linux_amd64.tar.gz && \
    tar xzf gotestsum_${GOTESTSUM_VERSION}_linux_amd64.tar.gz && \
    mv gotestsum /usr/local/bin/gotestsum && chmod +x /usr/local/bin/gotestsum && \
    rm gotestsum_${GOTESTSUM_VERSION}_linux_amd64.tar.gz

# Kind to run k8s cluster locally in Docker
RUN curl -fsSLO https://github.com/kubernetes-sigs/kind/releases/download/v${KIND_VERSION}/kind-linux-amd64 && \
    mv kind-linux-amd64 /usr/local/bin/kind && chmod +x /usr/local/bin/kind

# OpenShift installer and CLI to provision OCP clusters
RUN curl -fsSLO https://mirror.openshift.com/pub/openshift-v4/clients/ocp/${OPENSHIFT_TOOLS_VERSION}/openshift-install-linux-${OPENSHIFT_TOOLS_VERSION}.tar.gz && \
    tar -zxf openshift-install-linux-${OPENSHIFT_TOOLS_VERSION}.tar.gz openshift-install && \
    mv openshift-install /usr/local/bin/openshift-install && \
    rm openshift-install-linux-${OPENSHIFT_TOOLS_VERSION}.tar.gz && \
    curl -fsSLO https://mirror.openshift.com/pub/openshift-v4/clients/ocp/${OPENSHIFT_TOOLS_VERSION}/openshift-client-linux-${OPENSHIFT_TOOLS_VERSION}.tar.gz && \
    tar -zxf openshift-client-linux-${OPENSHIFT_TOOLS_VERSION}.tar.gz oc && \
    mv oc /usr/local/bin/oc && \
    rm openshift-client-linux-${OPENSHIFT_TOOLS_VERSION}.tar.gz

# Cache ECK Go dependencies in this Docker image
WORKDIR /go/src/github.com/elastic/cloud-on-k8s
COPY ["go.mod", "go.sum", "./"]
RUN go mod download && \
    rm /go/src/github.com/elastic/cloud-on-k8s/go.mod && \
    rm /go/src/github.com/elastic/cloud-on-k8s/go.sum
WORKDIR /
