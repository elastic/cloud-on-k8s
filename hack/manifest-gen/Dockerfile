FROM docker.elastic.co/wolfi/go:v1.26.3-r4@sha256:b373a2fbafa641e3db6d806505ddc6ea01b3ab3409e02efbc589eb2b4c813f79 as builder
ADD . /manifest-gen
WORKDIR /manifest-gen
ENV GO111MODULE=on CGO_ENABLED=0 GOOS=linux 
RUN go build -a -ldflags '-w -s' -o manifest-gen .

FROM scratch
COPY --from=builder /manifest-gen/manifest-gen /manifest-gen
COPY --from=builder /manifest-gen/assets /assets
ENTRYPOINT ["/manifest-gen"]
