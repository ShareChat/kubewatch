FROM golang:1.21-alpine AS builder 
RUN apk update && apk upgrade && apk add --no-cache git procps bash
RUN mkdir -p "$GOPATH/src/github.com/bitnami-labs/kubewatch"

ADD . "$GOPATH/src/github.com/bitnami-labs/kubewatch"

RUN cd "$GOPATH/src/github.com/bitnami-labs/kubewatch" && \
    go build -o /kubewatch
RUN echo 'http://dl-cdn.alpinelinux.org/alpine/edge/testing' >> /etc/apk/repositories
# These are needed for running filebeat
RUN apk add --no-cache curl libc6-compat

RUN cd "$GOPATH/src/github.com/bitnami-labs/kubewatch" && \
    cp /kubewatch /bin/kubewatch

ENV KW_CONFIG=/opt/bitnami/kubewatch
ARG GITHUB_TOKEN

RUN git config \
    --global \
    url."https://${GITHUB_TOKEN}@github.com".insteadOf \
    "https://github.com"
ARG DEPLOYMENT_ID
ENV DEPLOYMENT_ID ${DEPLOYMENT_ID}

ENTRYPOINT ["/bin/kubewatch"]
