ARG base=packs/base
ARG go_version=1.11.4

FROM golang:$go_version as helpers

RUN go get github.com/GoogleCloudPlatform/docker-credential-gcr
RUN go get github.com/awslabs/amazon-ecr-credential-helper/ecr-login/cli/docker-credential-ecr-login
RUN go get github.com/Azure/acr-docker-credential-helper/src/docker-credential-acr

FROM ${base}

COPY --from=helpers /go/bin /usr/local/bin
ARG stack_id="io.buildpacks.stacks.bionic"

RUN mkdir -p /workspace/app /workspace/config /cache /platform/env
RUN chown -R pack:pack /workspace /cache

USER pack
LABEL io.buildpacks.stack.id=${stack_id}
ENV PACK_STACK_ID=${stack_id}
