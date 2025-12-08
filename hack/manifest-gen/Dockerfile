FROM docker.elastic.co/wolfi/go:1.25.5-r0@sha256:425a55f42f4a86e8e7e39d066aecedfa4d3442d7be0ac0f4d27a7cc12bb115fb as builder
ADD . /manifest-gen
WORKDIR /manifest-gen
ENV GO111MODULE=on CGO_ENABLED=0 GOOS=linux 
RUN go build -a -ldflags '-w -s' -o manifest-gen .

FROM scratch
COPY --from=builder /manifest-gen/manifest-gen /manifest-gen
COPY --from=builder /manifest-gen/assets /assets
ENTRYPOINT ["/manifest-gen"]
