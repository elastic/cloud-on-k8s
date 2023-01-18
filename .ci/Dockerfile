# Docker image used for continuous integration
FROM docker.io/library/golang:1.19.5

ENV GOLANGCILINT_VERSION=1.48.0
ENV SHELLCHECK_VERSION=0.8.0
ENV GCLOUD_VERSION=381.0.0
ENV KUBECTL_VERSION=1.21.14
ENV DOCKER_VERSION=19.03.14
ENV DOCKER_BUILDX_VERSION=0.8.2
ENV GOTESTSUM_VERSION=1.8.0
ENV EKSCTL_VERSION=0.74.0
ENV ECK_DIAG_VERSION=1.2.0
ENV HELM_VERSION=3.9.1

# golangci-lint
RUN curl -sfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh \
    | sh -s -- -b $(go env GOPATH)/bin v${GOLANGCILINT_VERSION}

# etcd and apiserver for integration tests
ENV KUBEAPISERVER_VERSION=1.21.14
RUN curl -fsSL https://dl.k8s.io/v${KUBEAPISERVER_VERSION}/bin/linux/amd64/kube-apiserver -o /usr/local/bin/kube-apiserver && \
    chmod +x /usr/local/bin/kube-apiserver
ENV ETCD_VER=3.4.23
RUN curl -fsSLO https://storage.googleapis.com/etcd/v${ETCD_VER}/etcd-v${ETCD_VER}-linux-amd64.tar.gz && \
    tar zxf etcd-v${ETCD_VER}-linux-amd64.tar.gz -C /usr/local/bin --strip-components=1 && \
    rm -f etcd-v${ETCD_VER}-linux-amd64.tar.gz

# gcloud to provision GKE clusters
ENV PATH=${PATH}:/usr/local/google-cloud-sdk/bin
ENV USE_GKE_GCLOUD_AUTH_PLUGIN=True
RUN curl -fsSLO https://dl.google.com/dl/cloudsdk/channels/rapid/downloads/google-cloud-sdk-${GCLOUD_VERSION}-linux-x86_64.tar.gz && \
    mkdir -p /usr/local/gcloud && \
    tar -zxf google-cloud-sdk-${GCLOUD_VERSION}-linux-x86_64.tar.gz -C /usr/local && \
    /usr/local/google-cloud-sdk/install.sh --quiet && \
    gcloud config set core/disable_usage_reporting true && \
    gcloud config set component_manager/disable_update_check true && \
    gcloud components install beta gke-gcloud-auth-plugin --quiet && \
    rm google-cloud-sdk-${GCLOUD_VERSION}-linux-x86_64.tar.gz

# kubectl for deploying the operator and running e2e tests
RUN curl -fsSLO https://dl.k8s.io/v${KUBECTL_VERSION}/bin/linux/amd64/kubectl && \
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

# gotestsum to generate xUnit compatible XML for tests
RUN curl -fsSLO https://github.com/gotestyourself/gotestsum/releases/download/v${GOTESTSUM_VERSION}/gotestsum_${GOTESTSUM_VERSION}_linux_amd64.tar.gz && \
    tar xzf gotestsum_${GOTESTSUM_VERSION}_linux_amd64.tar.gz && \
    mv gotestsum /usr/local/bin/gotestsum && chmod +x /usr/local/bin/gotestsum && \
    rm gotestsum_${GOTESTSUM_VERSION}_linux_amd64.tar.gz

RUN curl -fsSLO https://github.com/elastic/eck-diagnostics/releases/download/${ECK_DIAG_VERSION}/eck-diagnostics_${ECK_DIAG_VERSION}_Linux_x86_64.tar.gz && \
    tar xzf eck-diagnostics_${ECK_DIAG_VERSION}_Linux_x86_64.tar.gz && \
    mv eck-diagnostics /usr/local/bin/eck-diagnostics

RUN curl -fsSLO "https://github.com/weaveworks/eksctl/releases/download/v${EKSCTL_VERSION}/eksctl_Linux_amd64.tar.gz" && \
    tar xzf eksctl_Linux_amd64.tar.gz && \
    mv eksctl /usr/local/bin/eksctl && \
    rm eksctl_Linux_amd64.tar.gz

RUN curl -fsSL -o get_helm.sh https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 && \
    chmod 700 get_helm.sh && \
    ./get_helm.sh -v v${HELM_VERSION} --no-sudo && \
    rm get_helm.sh

RUN helm plugin install https://github.com/quintush/helm-unittest --version 0.2.8

# Cache ECK Go dependencies in this Docker image
WORKDIR /go/src/github.com/elastic/cloud-on-k8s
COPY ["go.mod", "go.sum", "./"]
RUN go mod download && \
    rm /go/src/github.com/elastic/cloud-on-k8s/go.mod && \
    rm /go/src/github.com/elastic/cloud-on-k8s/go.sum
WORKDIR /
