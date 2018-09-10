ARG base=ubuntu:18.04
ARG go_version=1.11.0

FROM golang:${go_version} as builder

WORKDIR /go/src/github.com/buildpack/lifecycle
COPY . .
RUN CGO_ENABLED=0 go install -a -installsuffix static "./cmd/..."

RUN mv /go/bin /lifecycle && mkdir /go/bin

RUN go get github.com/sclevine/yj

FROM ${base}
ARG jq_url=http://stedolan.github.io/jq/download/linux64/jq

RUN apt-get update && \
  apt-get install -y wget xz-utils ca-certificates && \
  rm -rf /var/lib/apt/lists/*

RUN useradd -u 1000 -mU -s /bin/bash pack

COPY --from=builder /lifecycle /lifecycle
COPY --from=builder /go/bin /usr/local/bin

RUN wget -qO /usr/local/bin/jq "${jq_url}" && chmod +x /usr/local/bin/jq

WORKDIR /workspace
RUN chown -R pack:pack /workspace
