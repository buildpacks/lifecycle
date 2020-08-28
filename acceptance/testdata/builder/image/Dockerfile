FROM ubuntu:bionic

COPY container /

RUN chown -R 2222:3333 /layers
RUN chown -R 2222:3333 /cnb
RUN chown -R 2222:3333 /workspace

WORKDIR /layers

ENV CNB_USER_ID=2222

ENV CNB_GROUP_ID=3333
