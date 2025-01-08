FROM docker.elastic.co/wolfi/go:1.23.4@sha256:ce2112805a32a37095adbfe672e30089c2d394a42df7f982823d8934ace2da66 as builder
ADD . /manifest-gen
WORKDIR /manifest-gen
ENV GO111MODULE=on CGO_ENABLED=0 GOOS=linux 
RUN go build -a -ldflags '-w -s' -o manifest-gen .

FROM scratch
COPY --from=builder /manifest-gen/manifest-gen /manifest-gen
COPY --from=builder /manifest-gen/assets /assets
ENTRYPOINT ["/manifest-gen"]
