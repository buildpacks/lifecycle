# Stage 1
FROM ubuntu:bionic AS download-and-extract-lifecycle

ARG LIFECYCLE_URI

WORKDIR /tmp

RUN apt-get update && apt-get install -y wget
RUN wget $LIFECYCLE_URI -O lifecycle.tgz && tar xzvf lifecycle.tgz

RUN mkdir -p /cnb/lifecycle && mkdir /layers



# Stage 2
FROM gcr.io/distroless/static

COPY --from=download-and-extract-lifecycle /tmp/lifecycle /cnb/lifecycle

WORKDIR /layers
