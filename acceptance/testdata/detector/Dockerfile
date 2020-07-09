FROM ubuntu:bionic

ARG cnb_uid=1234
ARG cnb_gid=1000

COPY ./container/ /

ENTRYPOINT ["/cnb/lifecycle/detector"]

RUN groupadd cnb --gid ${cnb_gid} && \
  useradd --uid ${cnb_uid} --gid ${cnb_gid} -m -s /bin/bash cnb

RUN chown -R "${cnb_uid}:${cnb_gid}" "/layers"

WORKDIR /layers