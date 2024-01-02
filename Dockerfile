FROM golang AS builder
MAINTAINER "Cuong Manh Le <cuong.manhle.vn@gmail.com>"

RUN apt-get clean && \
    rm -rf /var/lib/apt/lists/* && \
    apt-get clean && \
    apt-get update && \
    apt-get upgrade && \
    dpkg --add-architecture arm64 &&\
    apt-get install -y --no-install-recommends build-essential && \
    mkdir -p "$GOPATH/src/github.com/bitnami-labs/kubewatch"

ADD . "$GOPATH/src/github.com/bitnami-labs/kubewatch"

RUN cd "$GOPATH/src/github.com/bitnami-labs/kubewatch" && \
    CGO_ENABLED=0 GOOS=linux GOARCH=$(dpkg --print-architecture) go build -a --installsuffix cgo --ldflags="-s" -o /kubewatch

FROM cgr.dev/chainguard/static:latest-glibc

COPY --from=builder /kubewatch /bin/kubewatch

ENV KW_CONFIG=/opt/bitnami/kubewatch

ENTRYPOINT ["/bin/kubewatch"]
