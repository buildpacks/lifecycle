FROM ubuntu:bionic
ARG cnb_platform_api

RUN apt-get update && apt-get install -y ca-certificates

COPY container /

WORKDIR /layers

ENV CNB_USER_ID=2222

ENV CNB_GROUP_ID=3333

ENV CNB_PLATFORM_API=${cnb_platform_api}

RUN chown -R $CNB_USER_ID:$CNB_GROUP_ID /some-dir

RUN chown -R $CNB_USER_ID:$CNB_GROUP_ID /layers

# ensure docker config directory is root owned and NOT world readable
RUN chown -R root /docker-config; chmod -R 700 /docker-config

ARG registry

# write some stack.toml files to use in tests
RUN echo "\
[run-image]\n\
 image = \"${registry}/company/stack:bionic\"\n\
 mirrors = []\n\
[build-image]\n\
 stack-id = \"io.buildpacks.stacks.bionic\"\n\
 mixins = []\n\
" > /cnb/stack.toml

RUN echo "\
[run-image]\n\
 image = \"${registry}/company/stack:centos\"\n\
 mirrors = []\n\
[build-image]\n\
 stack-id = \"io.buildpacks.stacks.bionic\"\n\
 mixins = []\n\
" > /cnb/mismatch-stack.toml

RUN echo "\
[run-image]\n\
 image = \"gcr.io/paketobuildpacks/invalidimg:20\"\n\
 mirrors = [\"${registry}/company/stack:bionic\"]\n\
[build-image]\n\
 stack-id = \"io.buildpacks.stacks.bionic\"\n\
 mixins = []\n\
" > /cnb/run-mirror-stack.toml

RUN echo "\
[run-image]\n\
 image = \"localcompany/stack:bionic\"\n\
 mirrors = []\n\
[build-image]\n\
 stack-id = \"io.buildpacks.stacks.bionic\"\n\
 mixins = []\n\
" > /cnb/local-bionic-stack.toml

RUN echo "[run-images" > /cnb/bad-stack.toml
