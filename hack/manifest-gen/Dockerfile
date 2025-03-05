FROM docker.elastic.co/wolfi/go:1.24.1@sha256:5126ac005e7da32ae92b37fc1c5f08c106353ac926a90f8d2ae77bd0f95524aa as builder
ADD . /manifest-gen
WORKDIR /manifest-gen
ENV GO111MODULE=on CGO_ENABLED=0 GOOS=linux 
RUN go build -a -ldflags '-w -s' -o manifest-gen .

FROM scratch
COPY --from=builder /manifest-gen/manifest-gen /manifest-gen
COPY --from=builder /manifest-gen/assets /assets
ENTRYPOINT ["/manifest-gen"]
