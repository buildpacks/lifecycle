ARG base=ubuntu:18.04
ARG go_version=1.11.0

FROM golang:${go_version} as builder

WORKDIR /go/src/github.com/buildpack/lifecycle
COPY . .
RUN CGO_ENABLED=0 GO111MODULE=on go install -a -installsuffix static "./cmd/..."

FROM ${base}
ARG jq_url=https://github.com/stedolan/jq/releases/download/jq-1.5/jq-linux64
ARG yj_url=https://github.com/sclevine/yj/releases/download/v2.0/yj-linux

LABEL io.buildpacks.stack.id="io.buildpacks.stacks.bionic"

RUN apt-get update && \
  apt-get install -y wget xz-utils ca-certificates && \
  rm -rf /var/lib/apt/lists/*

RUN useradd -u 1000 -mU -s /bin/bash pack

COPY --from=builder /go/bin /lifecycle

RUN wget -qO /usr/local/bin/jq "${jq_url}" && chmod +x /usr/local/bin/jq && \
  wget -qO /usr/local/bin/yj "${yj_url}" && chmod +x /usr/local/bin/yj

WORKDIR /workspace
RUN chown -R pack:pack /workspace
