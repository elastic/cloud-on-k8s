FROM docker.elastic.co/wolfi/go:1.23.3-r1@sha256:dddedbaba534d0dcedb52b76e93a0f285e20e2870ac3abf98c862ba1516c2f09 as builder
ADD . /manifest-gen
WORKDIR /manifest-gen
ENV GO111MODULE=on CGO_ENABLED=0 GOOS=linux 
RUN go build -a -ldflags '-w -s' -o manifest-gen .

FROM scratch
COPY --from=builder /manifest-gen/manifest-gen /manifest-gen
COPY --from=builder /manifest-gen/assets /assets
ENTRYPOINT ["/manifest-gen"]
