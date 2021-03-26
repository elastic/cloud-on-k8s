FROM buildpack-deps:bullseye-curl as builder
ARG VERSION


# OpenShift installer and CLI to provision OCP clusters
RUN curl -fsSLO https://mirror.openshift.com/pub/openshift-v4/clients/ocp/${VERSION}/openshift-install-linux-${VERSION}.tar.gz && \
    tar -zxf openshift-install-linux-${VERSION}.tar.gz openshift-install && \
    mv openshift-install /usr/local/bin/openshift-install && \
    rm openshift-install-linux-${VERSION}.tar.gz && \
    curl -fsSLO https://mirror.openshift.com/pub/openshift-v4/clients/ocp/${VERSION}/openshift-client-linux-${VERSION}.tar.gz && \
    tar -zxf openshift-client-linux-${VERSION}.tar.gz oc && \
    mv oc /usr/local/bin/oc && \
    rm openshift-client-linux-${VERSION}.tar.gz

FROM scratch
# be able to make TLS requests to the GCloud API
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /usr/local/bin/openshift-install .
COPY --from=builder /usr/local/bin/oc .



