FROM ubuntu:bionic

RUN apt-get update && apt-get install -y ca-certificates

COPY container /

WORKDIR /layers

ENV CNB_USER_ID=2222

ENV CNB_GROUP_ID=3333
