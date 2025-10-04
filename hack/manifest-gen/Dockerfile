FROM docker.elastic.co/wolfi/go:1.25.1@sha256:5e1b1229dd49b1755e9506d31fe0897c4a14bbc8fdd89d1952e3a9a47da7a8af as builder
ADD . /manifest-gen
WORKDIR /manifest-gen
ENV GO111MODULE=on CGO_ENABLED=0 GOOS=linux 
RUN go build -a -ldflags '-w -s' -o manifest-gen .

FROM scratch
COPY --from=builder /manifest-gen/manifest-gen /manifest-gen
COPY --from=builder /manifest-gen/assets /assets
ENTRYPOINT ["/manifest-gen"]
