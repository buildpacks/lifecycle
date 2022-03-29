FROM golang:1.17 as builder

COPY exec.d/ /go/src/exec.d
RUN cd ./src/exec.d && go build -o /go/helper

FROM ubuntu:bionic

COPY linux/container /

RUN rm /layers/0.5_buildpack/some_layer/exec.d/exec.d-checker/.gitkeep
COPY --from=builder /go/helper /layers/0.5_buildpack/some_layer/exec.d/helper
COPY --from=builder /go/helper /layers/0.5_buildpack/some_layer/exec.d/exec.d-checker/helper

ENV PATH="/cnb/process:/cnb/lifecycle:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"

ENTRYPOINT ["/cnb/lifecycle/launcher"]
