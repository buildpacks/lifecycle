FROM ubuntu:bionic

ARG cnb_uid=1234
ARG cnb_gid=1000

ENV CNB_USER_ID=${cnb_uid}
ENV CNB_GROUP_ID=${cnb_gid}

COPY ./container/ /

RUN groupadd cnb --gid ${cnb_gid} && \
  useradd --uid ${cnb_uid} --gid ${cnb_gid} -m -s /bin/bash cnb
