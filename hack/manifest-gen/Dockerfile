FROM docker.elastic.co/wolfi/go:1.25.7-r0@sha256:8500bdb0a502998b1beeca1347e0938d64d2fc61f93865960e566f8246d2c6c3 as builder
ADD . /manifest-gen
WORKDIR /manifest-gen
ENV GO111MODULE=on CGO_ENABLED=0 GOOS=linux 
RUN go build -a -ldflags '-w -s' -o manifest-gen .

FROM scratch
COPY --from=builder /manifest-gen/manifest-gen /manifest-gen
COPY --from=builder /manifest-gen/assets /assets
ENTRYPOINT ["/manifest-gen"]
