FROM centos:7

RUN yum -y install nc

ENV KUBECTL_VERSION 1.13.2
RUN curl -sSL https://storage.googleapis.com/kubernetes-release/release/v${KUBECTL_VERSION}/bin/linux/amd64/kubectl \
    > /usr/local/bin/kubectl && chmod +x /usr/local/bin/kubectl
