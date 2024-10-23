FROM ubuntu:bionic

ARG cnb_uid=1234
ARG cnb_gid=1000

ENV CNB_USER_ID=${cnb_uid}
ENV CNB_GROUP_ID=${cnb_gid}

COPY ./container/ /

# We have to pre-create the tar files so that their digests do not change due to timestamps
# But, ':' in the filepath on Windows is not allowed
RUN mv /cache/committed/sha256_2d9c9c638d5c4f0df067eeae7b9c99ad05776a89d19ab863c28850a91e5f2944.tar /cache/committed/sha256:2d9c9c638d5c4f0df067eeae7b9c99ad05776a89d19ab863c28850a91e5f2944.tar
RUN mv /cache/committed/sha256_430338f576c11e5236669f9c843599d96afe28784cffcb2d46ddb07beb00df78.tar /cache/committed/sha256:430338f576c11e5236669f9c843599d96afe28784cffcb2d46ddb07beb00df78.tar

ENTRYPOINT ["/cnb/lifecycle/restorer"]

RUN groupadd cnb --gid ${cnb_gid} && \
  useradd --uid ${cnb_uid} --gid ${cnb_gid} -m -s /bin/bash cnb

# chown the directories so the restorer tests do not have to run as root
RUN chown -R "${cnb_uid}:${cnb_gid}" "/layers"
RUN chown -R "${cnb_uid}:${cnb_gid}" "/cache"
RUN chown -R "${cnb_uid}:${cnb_gid}" "/kaniko"

WORKDIR /layers

USER ${cnb_uid}:${cnb_gid}
