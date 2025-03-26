FROM docker.elastic.co/wolfi/go:1.24.1-r2@sha256:6e18c3fd770a781fe68c823c64ba7deae0a73dbc50dbe8e579224563892e50c5 as builder
ADD . /manifest-gen
WORKDIR /manifest-gen
ENV GO111MODULE=on CGO_ENABLED=0 GOOS=linux 
RUN go build -a -ldflags '-w -s' -o manifest-gen .

FROM scratch
COPY --from=builder /manifest-gen/manifest-gen /manifest-gen
COPY --from=builder /manifest-gen/assets /assets
ENTRYPOINT ["/manifest-gen"]
