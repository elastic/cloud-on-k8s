FROM docker.elastic.co/wolfi/go:1.24.1-r1@sha256:192dc9849d88aba6cb7b56ef37ae710c59ace4ee2a3447b93b893d016b4cd990 as builder
ADD . /manifest-gen
WORKDIR /manifest-gen
ENV GO111MODULE=on CGO_ENABLED=0 GOOS=linux 
RUN go build -a -ldflags '-w -s' -o manifest-gen .

FROM scratch
COPY --from=builder /manifest-gen/manifest-gen /manifest-gen
COPY --from=builder /manifest-gen/assets /assets
ENTRYPOINT ["/manifest-gen"]
