# Stage 1
FROM ubuntu:bionic AS download-and-extract-lifecycle

ARG LIFECYCLE_URI

WORKDIR /tmp

RUN apt-get update && apt-get install -y wget

RUN wget $LIFECYCLE_URI -O lifecycle.tgz && tar xzvf lifecycle.tgz


# Stage 2
FROM gcr.io/distroless/static

ARG LIFECYCLE_VERSION
ARG BUILDPACK_API
ARG PLATFORM_API

COPY --from=download-and-extract-lifecycle /tmp/lifecycle /cnb/lifecycle

LABEL io.buildpacks.builder.metadata="{\"lifecycle\":{\"version\":\"$LIFECYCLE_VERSION\",\"api\":{\"buildpack\":\"$BUILDPACK_API\",\"platform\":\"$PLATFORM_API\"}}}"

WORKDIR /layers
