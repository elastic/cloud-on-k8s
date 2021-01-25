# Docker image used for continuous integration
FROM golang:1.15.7

ENV GOLANGCILINT_VERSION=1.27.0
ENV SHELLCHECK_VERSION=0.7.1
ENV KUBEBUILDER_VERSION=2.3.1
ENV GCLOUD_VERSION=297.0.1
ENV KUBECTL_VERSION=1.14.7
ENV DOCKER_VERSION=19.03.13
ENV DOCKER_BUILDX_VERSION=0.4.2
ENV GOTESTSUM_VERSION=0.5.0
ENV KIND_VERSION_0_8=0.8.1
ENV KIND_VERSION_0_9=0.9.0
ENV OPENSHIFT_TOOLS_VERSION=4.3.19
ENV EKSCTL_VERSION=0.34.0

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

# Docker buildx extension for building multi-arch images
RUN mkdir -p ~/.docker/cli-plugins && \ 
    curl -fsSLo ~/.docker/cli-plugins/docker-buildx https://github.com/docker/buildx/releases/download/v${DOCKER_BUILDX_VERSION}/buildx-v${DOCKER_BUILDX_VERSION}.linux-amd64 && \
    chmod a+x ~/.docker/cli-plugins/docker-buildx

# xz-utils to decompress shellcheck, unzip for aws-cli, qemu-system-arm and qemu-user-static for multi-arch
RUN apt-get update && apt-get --no-install-recommends -y install \
    unzip xz-utils qemu-system-arm qemu-user-static && \
    apt-get clean && apt-get autoclean && \
    rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*

# AWS CLI v2 to upload YAML on S3 and run eksctl
RUN curl -sSL "https://awscli.amazonaws.com/awscli-exe-linux-x86_64.zip" -o "awscliv2.zip" && \
    unzip -q awscliv2.zip && \
    ./aws/install && \
    rm awscliv2.zip

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

# Kind to run k8s cluster locally in Docker, we need to have both 0.8.x and 0.9.x to be able to test older k8s versions
RUN curl -fsSLO https://github.com/kubernetes-sigs/kind/releases/download/v${KIND_VERSION_0_8}/kind-linux-amd64 && \
    mv kind-linux-amd64 /usr/local/bin/kind-${KIND_VERSION_0_8} && chmod +x /usr/local/bin/kind-${KIND_VERSION_0_8}

RUN curl -fsSLO https://github.com/kubernetes-sigs/kind/releases/download/v${KIND_VERSION_0_9}/kind-linux-amd64 && \
    mv kind-linux-amd64 /usr/local/bin/kind-${KIND_VERSION_0_9} && chmod +x /usr/local/bin/kind-${KIND_VERSION_0_9}

# OpenShift installer and CLI to provision OCP clusters
RUN curl -fsSLO https://mirror.openshift.com/pub/openshift-v4/clients/ocp/${OPENSHIFT_TOOLS_VERSION}/openshift-install-linux-${OPENSHIFT_TOOLS_VERSION}.tar.gz && \
    tar -zxf openshift-install-linux-${OPENSHIFT_TOOLS_VERSION}.tar.gz openshift-install && \
    mv openshift-install /usr/local/bin/openshift-install && \
    rm openshift-install-linux-${OPENSHIFT_TOOLS_VERSION}.tar.gz && \
    curl -fsSLO https://mirror.openshift.com/pub/openshift-v4/clients/ocp/${OPENSHIFT_TOOLS_VERSION}/openshift-client-linux-${OPENSHIFT_TOOLS_VERSION}.tar.gz && \
    tar -zxf openshift-client-linux-${OPENSHIFT_TOOLS_VERSION}.tar.gz oc && \
    mv oc /usr/local/bin/oc && \
    rm openshift-client-linux-${OPENSHIFT_TOOLS_VERSION}.tar.gz

RUN curl -fsSLO "https://github.com/weaveworks/eksctl/releases/download/${EKSCTL_VERSION}/eksctl_Linux_amd64.tar.gz" && \
    tar xzf eksctl_Linux_amd64.tar.gz && \
    mv eksctl /usr/local/bin/eksctl && \
    rm eksctl_Linux_amd64.tar.gz

# Cache ECK Go dependencies in this Docker image
WORKDIR /go/src/github.com/elastic/cloud-on-k8s
COPY ["go.mod", "go.sum", "./"]
RUN go mod download && \
    rm /go/src/github.com/elastic/cloud-on-k8s/go.mod && \
    rm /go/src/github.com/elastic/cloud-on-k8s/go.sum
WORKDIR /
