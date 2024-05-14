FROM ubuntu:jammy

ARG cnb_uid=1234
ARG cnb_gid=1000
ARG cnb_platform_api

ENV CNB_PLATFORM_API=${cnb_platform_api}

COPY ./container/ /

ENTRYPOINT ["/cnb/lifecycle/detector"]

RUN groupadd cnb --gid ${cnb_gid} && \
  useradd --uid ${cnb_uid} --gid ${cnb_gid} -m -s /bin/bash cnb

RUN chown -R "${cnb_uid}:${cnb_gid}" "/layers"

WORKDIR /layers

USER ${cnb_uid}:${cnb_gid}
