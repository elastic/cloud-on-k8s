# Docker image used for continuous integration
# Fedora because it ships a recent version of buildah (>=1.26.0).
FROM fedora:36

# Base packages required
RUN dnf -y install ca-certificates openssl git make gcc

# gcloud to provision GKE clusters
ENV GCLOUD_VERSION=381.0.0
ENV PATH=${PATH}:/usr/local/google-cloud-sdk/bin
RUN curl -fsSLO https://dl.google.com/dl/cloudsdk/channels/rapid/downloads/google-cloud-sdk-${GCLOUD_VERSION}-linux-x86_64.tar.gz && \
    mkdir -p /usr/local/gcloud && \
    tar -zxf google-cloud-sdk-${GCLOUD_VERSION}-linux-x86_64.tar.gz -C /usr/local && \
    /usr/local/google-cloud-sdk/install.sh --quiet && \
    gcloud config set core/disable_usage_reporting true && \
    gcloud config set component_manager/disable_update_check true && \
    gcloud components install beta --quiet && \
    rm google-cloud-sdk-${GCLOUD_VERSION}-linux-x86_64.tar.gz

# AWS CLI v2 to upload YAML on S3 and use eksctl
RUN dnf -y install unzip && \
    curl -sSL "https://awscli.amazonaws.com/awscli-exe-linux-x86_64.zip" -o "awscliv2.zip" && \
    unzip -q awscliv2.zip && \
    ./aws/install && \
    rm awscliv2.zip

# eksctl to provision Amazon EKS clusters
ENV EKSCTL_VERSION=0.74.0
RUN curl -fsSLO "https://github.com/weaveworks/eksctl/releases/download/v${EKSCTL_VERSION}/eksctl_Linux_amd64.tar.gz" && \
    tar xzf eksctl_Linux_amd64.tar.gz && \
    mv eksctl /usr/local/bin/eksctl && \
    rm eksctl_Linux_amd64.tar.gz

# kubebuilder to get etcd, apiserver, etc. binaries for integration tests
ENV KUBEBUILDER_VERSION=2.3.1
ENV PATH=${PATH}:/usr/local/kubebuilder/bin
RUN curl -fsSLO https://github.com/kubernetes-sigs/kubebuilder/releases/download/v${KUBEBUILDER_VERSION}/kubebuilder_${KUBEBUILDER_VERSION}_linux_amd64.tar.gz && \
    tar -zxf kubebuilder_${KUBEBUILDER_VERSION}_linux_amd64.tar.gz && \
    mv kubebuilder_${KUBEBUILDER_VERSION}_linux_amd64 /usr/local/kubebuilder && \
    rm kubebuilder_${KUBEBUILDER_VERSION}_linux_amd64.tar.gz

# kubectl for deploying the operator and running e2e tests
ENV KUBECTL_VERSION=1.19.16
RUN curl -fsSLO https://storage.googleapis.com/kubernetes-release/release/v${KUBECTL_VERSION}/bin/linux/amd64/kubectl && \
    mv kubectl /usr/local/bin/kubectl && chmod +x /usr/local/bin/kubectl

# Helm
ENV HELM_VERSION=3.9.1
RUN curl -fsSL -o get_helm.sh https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 && \
    chmod 700 get_helm.sh && \
    ./get_helm.sh -v v${HELM_VERSION} --no-sudo && \
    rm get_helm.sh
RUN helm plugin install https://github.com/quintush/helm-unittest --version 0.2.8

# Buildah to build and push images on k8s
RUN dnf install -y buildah

# Docker client to build and push images
ENV DOCKER_VERSION=19.03.14
RUN curl -fsSLO https://download.docker.com/linux/static/stable/x86_64/docker-${DOCKER_VERSION}.tgz && \
    tar xzf docker-${DOCKER_VERSION}.tgz --strip 1 -C /usr/local/bin docker/docker && \
    rm docker-${DOCKER_VERSION}.tgz

# Docker buildx extension for building multi-arch images
RUN dnf -y install qemu-system-arm qemu-user-static
ENV DOCKER_BUILDX_VERSION=0.8.2
RUN mkdir -p ~/.docker/cli-plugins && \
    curl -fsSLo ~/.docker/cli-plugins/docker-buildx https://github.com/docker/buildx/releases/download/v${DOCKER_BUILDX_VERSION}/buildx-v${DOCKER_BUILDX_VERSION}.linux-amd64 && \
    chmod a+x ~/.docker/cli-plugins/docker-buildx

# shellcheck
ENV SHELLCHECK_VERSION=0.8.0
ENV PATH=${PATH}:/usr/local/shellcheck
RUN curl -fsSLO https://github.com/koalaman/shellcheck/releases/download/v${SHELLCHECK_VERSION}/shellcheck-v${SHELLCHECK_VERSION}.linux.x86_64.tar.xz && \
    tar -xf shellcheck-v${SHELLCHECK_VERSION}.linux.x86_64.tar.xz && \
    mv shellcheck-v${SHELLCHECK_VERSION} /usr/local/shellcheck && \
    rm shellcheck-v${SHELLCHECK_VERSION}.linux.x86_64.tar.xz

# gotestsum to generate xUnit compatible XML for tests
ENV GOTESTSUM_VERSION=1.8.0
RUN curl -fsSLO https://github.com/gotestyourself/gotestsum/releases/download/v${GOTESTSUM_VERSION}/gotestsum_${GOTESTSUM_VERSION}_linux_amd64.tar.gz && \
    tar xzf gotestsum_${GOTESTSUM_VERSION}_linux_amd64.tar.gz && \
    mv gotestsum /usr/local/bin/gotestsum && chmod +x /usr/local/bin/gotestsum && \
    rm gotestsum_${GOTESTSUM_VERSION}_linux_amd64.tar.gz

# golangci-lint
ENV GOLANGCILINT_VERSION=1.48.0
RUN curl -sfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh \
    | sh -s -- -b $(go env GOPATH)/bin v${GOLANGCILINT_VERSION}

# Golang
ENV GO_VERSION=1.19.1
RUN curl -fsSLO "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" && \
    tar -C /usr/local -xzf go${GO_VERSION}.linux-amd64.tar.gz
ENV GOPATH /go
ENV PATH /usr/local/go/bin:${PATH}

# eck-diagnostics
ENV ECK_DIAG_VERSION=1.0.3
RUN curl -fsSLO https://github.com/elastic/eck-diagnostics/releases/download/${ECK_DIAG_VERSION}/eck-diagnostics_${ECK_DIAG_VERSION}_Linux_x86_64.tar.gz && \
    tar xzf eck-diagnostics_${ECK_DIAG_VERSION}_Linux_x86_64.tar.gz && \
    mv eck-diagnostics /usr/local/bin/eck-diagnostics

RUN git config --global --add safe.directory '/go/src/github.com/elastic/cloud-on-k8s'
WORKDIR /go/src/github.com/elastic
