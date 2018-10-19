ARG go_version=1.10.3
ARG base=packs/base

FROM golang:$go_version as helpers

RUN go get github.com/GoogleCloudPlatform/docker-credential-gcr
RUN go get github.com/awslabs/amazon-ecr-credential-helper/ecr-login/cli/docker-credential-ecr-login
RUN go get github.com/Azure/acr-docker-credential-helper/src/docker-credential-acr

FROM ${base}

COPY --from=helpers /go/bin /usr/local/bin

USER pack
