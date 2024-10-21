FROM ubuntu:bionic

ARG cnb_uid=1234
ARG cnb_gid=1000

ENV CNB_USER_ID=${cnb_uid}
ENV CNB_GROUP_ID=${cnb_gid}

COPY ./container/ /

# turn /to_cache/<buildpack> directories into cache tarballs
# these are referenced by sha in /cache/committed/io.buildpacks.lifecycle.cache.metadata
RUN tar cvf /cache/committed/sha256:2d9c9c638d5c4f0df067eeae7b9c99ad05776a89d19ab863c28850a91e5f2944.tar -C /to_cache/cacher_buildpack layers
RUN tar cvf /cache/committed/sha256:58bafa1e79c8e44151141c95086beb37ca85b69578fc890bce33bb4c6c8e851f.tar -C /to_cache/unused_buildpack layers
# this layer has the wrong sha to mimic corrupted data
RUN tar cvf /cache/committed/sha256:b5bb9d8014a0f9b1d61e21e796d78dccdf1352f23cd32812f4850b878ae4944c.tar -C /to_cache/corrupted_buildpack layers

ENTRYPOINT ["/cnb/lifecycle/restorer"]

RUN groupadd cnb --gid ${cnb_gid} && \
  useradd --uid ${cnb_uid} --gid ${cnb_gid} -m -s /bin/bash cnb

# chown the directories so the restorer tests do not have to run as root
RUN chown -R "${cnb_uid}:${cnb_gid}" "/layers"
RUN chown -R "${cnb_uid}:${cnb_gid}" "/cache"
RUN chown -R "${cnb_uid}:${cnb_gid}" "/kaniko"

WORKDIR /layers

USER ${cnb_uid}:${cnb_gid}
