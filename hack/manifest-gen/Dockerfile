FROM docker.elastic.co/wolfi/go:1.23.3-r1@sha256:afb2ab25109c68f4f508ea84b3ace72d793816ba983453d8792de3d4eef6b1f5 as builder
ADD . /manifest-gen
WORKDIR /manifest-gen
ENV GO111MODULE=on CGO_ENABLED=0 GOOS=linux 
RUN go build -a -ldflags '-w -s' -o manifest-gen .

FROM scratch
COPY --from=builder /manifest-gen/manifest-gen /manifest-gen
COPY --from=builder /manifest-gen/assets /assets
ENTRYPOINT ["/manifest-gen"]
