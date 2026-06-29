FROM docker.elastic.co/wolfi/go:v1.26.4-r1@sha256:84e66c2579e815690d8ed58d65bbad3b72ba873ae30f39da08fb7e8f3482cad1 as builder
ADD . /manifest-gen
WORKDIR /manifest-gen
ENV GO111MODULE=on CGO_ENABLED=0 GOOS=linux 
RUN go build -a -ldflags '-w -s' -o manifest-gen .

FROM scratch
COPY --from=builder /manifest-gen/manifest-gen /manifest-gen
COPY --from=builder /manifest-gen/assets /assets
ENTRYPOINT ["/manifest-gen"]
