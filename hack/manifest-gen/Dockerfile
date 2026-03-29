FROM docker.elastic.co/wolfi/go:1.26.1-r1@sha256:3c96d63c2a711da154d7e00f24f7120ef93e1f60fb44caf5da693eb044d62462 as builder
ADD . /manifest-gen
WORKDIR /manifest-gen
ENV GO111MODULE=on CGO_ENABLED=0 GOOS=linux 
RUN go build -a -ldflags '-w -s' -o manifest-gen .

FROM scratch
COPY --from=builder /manifest-gen/manifest-gen /manifest-gen
COPY --from=builder /manifest-gen/assets /assets
ENTRYPOINT ["/manifest-gen"]
