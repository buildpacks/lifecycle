# Stage 1
FROM ubuntu:bionic AS download-and-extract-lifecycle

ARG LIFECYCLE_VERSION
ARG LIFECYCLE_GITHUB_ORG
ARG LIFECYCLE_TGZ=lifecycle-v${LIFECYCLE_VERSION}+linux.x86-64.tgz
ARG LIFECYCLE_URI="https://github.com/${LIFECYCLE_GITHUB_ORG}/lifecycle/releases/download/v${LIFECYCLE_VERSION}/${LIFECYCLE_TGZ}"

WORKDIR /tmp

RUN apt-get update && apt-get install -y wget
RUN wget $LIFECYCLE_URI && tar xzvf $LIFECYCLE_TGZ

RUN mkdir -p /cnb/lifecycle && mkdir /layers


VOLUME /tmp/lifecycle


# Stage 2
FROM gcr.io/distroless/static

WORKDIR /cnb/lifecycle

COPY --from=download-and-extract-lifecycle /tmp/lifecycle/* ./

WORKDIR /layers
