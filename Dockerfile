FROM golang:1.19-alpine AS builder 
RUN apk update && apk upgrade && apk add --no-cache git procps bash
RUN dpkg --add-architecture arm64 &&\
    mkdir -p "$GOPATH/src/github.com/bitnami-labs/kubewatch"

ADD . "$GOPATH/src/github.com/bitnami-labs/kubewatch"

RUN cd "$GOPATH/src/github.com/bitnami-labs/kubewatch" && \
    CGO_ENABLED=0 GOOS=linux GOARCH=$(dpkg --print-architecture) go build -a --installsuffix cgo --ldflags="-s" -o /kubewatch
RUN echo 'http://dl-cdn.alpinelinux.org/alpine/edge/testing' >> /etc/apk/repositories
# These are needed for running filebeat
RUN apk add --no-cache curl libc6-compat

FROM cgr.dev/chainguard/static:latest-glibc

COPY --from=builder /kubewatch /bin/kubewatch

ENV KW_CONFIG=/opt/bitnami/kubewatch

ENTRYPOINT ["/bin/kubewatch"]
