FROM ubuntu:bionic

COPY container /

RUN mkdir -p /cnb
COPY buildpacks /cnb/buildpacks
COPY group.toml /cnb/
COPY plan.toml /cnb/

WORKDIR /layers

ENV CNB_USER_ID=2222

ENV CNB_GROUP_ID=3333
