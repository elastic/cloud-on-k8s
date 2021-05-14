FROM buildpack-deps:bullseye-curl as builder

ARG CLIENT_VERSION
ENV DOCKER_VERSION=19.03.13

# Docker client to build and push images
RUN curl -fsSLO https://download.docker.com/linux/static/stable/x86_64/docker-${DOCKER_VERSION}.tgz && \
    tar xzf docker-${DOCKER_VERSION}.tgz --strip 1 -C /usr/local/bin docker/docker && \
    rm docker-${DOCKER_VERSION}.tgz
# Kind to run k8s cluster locally in Docker
RUN curl -fsSLO https://github.com/kubernetes-sigs/kind/releases/download/v${CLIENT_VERSION}/kind-linux-amd64 && \
    mv kind-linux-amd64 /usr/local/bin/kind && chmod +x /usr/local/bin/kind

FROM scratch

COPY --from=builder /usr/local/bin/kind .
COPY --from=builder /usr/local/bin/docker .