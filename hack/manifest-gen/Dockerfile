FROM docker.elastic.co/wolfi/go:1.26.1-r1@sha256:c7fa4e40bd46cf3ba503b9a300bfd118cfd03af94b4b1ee74922c5619f53c2bf as builder
ADD . /manifest-gen
WORKDIR /manifest-gen
ENV GO111MODULE=on CGO_ENABLED=0 GOOS=linux 
RUN go build -a -ldflags '-w -s' -o manifest-gen .

FROM scratch
COPY --from=builder /manifest-gen/manifest-gen /manifest-gen
COPY --from=builder /manifest-gen/assets /assets
ENTRYPOINT ["/manifest-gen"]
