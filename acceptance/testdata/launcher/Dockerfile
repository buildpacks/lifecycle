FROM golang:1.19 as builder

COPY exec.d/ /go/src/exec.d
RUN GO111MODULE=off go build -o helper ./src/exec.d

FROM ubuntu:bionic

COPY linux/container /

RUN rm /layers/0.5_buildpack/some_layer/exec.d/exec.d-checker/.gitkeep
COPY --from=builder /go/helper /layers/0.5_buildpack/some_layer/exec.d/helper
COPY --from=builder /go/helper /layers/0.5_buildpack/some_layer/exec.d/exec.d-checker/helper

ENV PATH="/cnb/process:/cnb/lifecycle:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"

RUN ln -s /cnb/lifecycle/launcher /cnb/process/exec.d-checker  # register process type for newer platform API

ENTRYPOINT ["/cnb/lifecycle/launcher"]
