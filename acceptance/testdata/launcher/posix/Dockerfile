FROM golang:1.15 as builder

COPY exec.d/ /go/src/exec.d
RUN go build -o helper ./src/exec.d


FROM ubuntu:bionic

COPY container /

RUN rm /layers/0.5_buildpack/some_layer/exec.d/exec.d-checker/.gitkeep
COPY --from=builder /go/helper /layers/0.5_buildpack/some_layer/exec.d/helper
COPY --from=builder /go/helper /layers/0.5_buildpack/some_layer/exec.d/exec.d-checker/helper

ENV PATH="/cnb/process:/cnb/lifecycle:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"

ENTRYPOINT ["/cnb/lifecycle/launcher"]
