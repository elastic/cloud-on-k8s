FROM docker.elastic.co/wolfi/go:1.25.3-r0@sha256:d79c47f4cbbda2316a4903e689e9d749c010148fb36b0c3786a86480e863ca1b as builder
ADD . /manifest-gen
WORKDIR /manifest-gen
ENV GO111MODULE=on CGO_ENABLED=0 GOOS=linux 
RUN go build -a -ldflags '-w -s' -o manifest-gen .

FROM scratch
COPY --from=builder /manifest-gen/manifest-gen /manifest-gen
COPY --from=builder /manifest-gen/assets /assets
ENTRYPOINT ["/manifest-gen"]
