# Docker image aimed to run compilation and tests in CI
FROM golang:1.11

ENV KUBEBUILDER_VERSION=1.0.6
ENV DOCKER_VERSION=18.03.1-ce
ENV DOCKER_API_VERSION=1.23

# Download kubebuilder release to get required tools (etcd, apiserver, etc.)
RUN curl -L -O https://github.com/kubernetes-sigs/kubebuilder/releases/download/v${KUBEBUILDER_VERSION}/kubebuilder_${KUBEBUILDER_VERSION}_linux_amd64.tar.gz && \
    tar -zxvf kubebuilder_${KUBEBUILDER_VERSION}_linux_amd64.tar.gz && \
    mv kubebuilder_${KUBEBUILDER_VERSION}_linux_amd64 /usr/local/kubebuilder
# update PATH to include downloaded binaries
ENV PATH=${PATH}:/usr/local/kubebuilder/bin

# Download required golang tools
RUN go get github.com/golang/dep/cmd/dep golang.org/x/tools/cmd/goimports

# Install Docker client for building and pushing images
RUN curl -fsSLO https://download.docker.com/linux/static/stable/x86_64/docker-${DOCKER_VERSION}.tgz \
  && tar xzvf docker-${DOCKER_VERSION}.tgz --strip 1 \
                 -C /usr/local/bin docker/docker \
  && rm docker-${DOCKER_VERSION}.tgz
