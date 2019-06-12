# Docker image aimed to run compilation and tests in CI
FROM golang:1.11

ENV KUBEBUILDER_VERSION=1.0.8
ENV GCLOUD_VERSION=232.0.0
ENV KUBECTL_VERSION=1.13.2
ENV DOCKER_VERSION=18.03.1-ce

# Download kubebuilder release to get required tools (etcd, apiserver, etc.)
ENV PATH=${PATH}:/usr/local/kubebuilder/bin
RUN curl -fsSLO https://github.com/kubernetes-sigs/kubebuilder/releases/download/v${KUBEBUILDER_VERSION}/kubebuilder_${KUBEBUILDER_VERSION}_linux_amd64.tar.gz && \
    tar -zxf kubebuilder_${KUBEBUILDER_VERSION}_linux_amd64.tar.gz && \
    mv kubebuilder_${KUBEBUILDER_VERSION}_linux_amd64 /usr/local/kubebuilder

# Download required golang tools
RUN go get github.com/golang/dep/cmd/dep golang.org/x/tools/cmd/goimports

# Download gcloud for provisioning gke clusters
ENV PATH=${PATH}:/usr/local/google-cloud-sdk/bin
RUN curl -fsSLO https://dl.google.com/dl/cloudsdk/channels/rapid/downloads/google-cloud-sdk-${GCLOUD_VERSION}-linux-x86_64.tar.gz && \
    mkdir -p /usr/local/gcloud && \
    tar -zxf google-cloud-sdk-${GCLOUD_VERSION}-linux-x86_64.tar.gz -C /usr/local && \
    /usr/local/google-cloud-sdk/install.sh && \
    gcloud config set core/disable_usage_reporting true && \
    gcloud config set component_manager/disable_update_check true && \
    gcloud components install beta --quiet

# Download kubectl for deploying the operator and running e2e tests
RUN curl -fsSLO https://storage.googleapis.com/kubernetes-release/release/v${KUBECTL_VERSION}/bin/linux/amd64/kubectl && \
    mv kubectl /usr/local/bin/kubectl && chmod +x /usr/local/bin/kubectl

# Install Docker client for building and pushing images
RUN curl -fsSLO https://download.docker.com/linux/static/stable/x86_64/docker-${DOCKER_VERSION}.tgz && \
    tar xzf docker-${DOCKER_VERSION}.tgz --strip 1 -C /usr/local/bin docker/docker && \
    rm docker-${DOCKER_VERSION}.tgz

# Install AWS CLI
RUN apt-get update && apt-get --no-install-recommends -y install \
    awscli && \
    apt-get clean && apt-get autoclean && \
    rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*
