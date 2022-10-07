FROM ubuntu:bionic

ARG cnb_uid=1234
ARG cnb_gid=1000

ENV CNB_USER_ID=${cnb_uid}
ENV CNB_GROUP_ID=${cnb_gid}

COPY ./container/ /

# turn /to_cache/<buildpack> directories into cache tarballs
# these are referenced by sha in /cache/committed/io.buildpacks.lifecycle.cache.metadata
RUN tar cvf /cache/committed/sha256:b89860e2f9c62e6b5d66d3ce019e18cdabae30273c25150b7f20a82f7a70e494.tar -C /to_cache/cacher_buildpack layers
RUN tar cvf /cache/committed/sha256:58bafa1e79c8e44151141c95086beb37ca85b69578fc890bce33bb4c6c8e851f.tar -C /to_cache/unused_buildpack layers

ENTRYPOINT ["/cnb/lifecycle/restorer"]

RUN groupadd cnb --gid ${cnb_gid} && \
  useradd --uid ${cnb_uid} --gid ${cnb_gid} -m -s /bin/bash cnb

# chown the directories so the restorer tests do not have to run as root
RUN chown -R "${cnb_uid}:${cnb_gid}" "/layers"
RUN chown -R "${cnb_uid}:${cnb_gid}" "/cache"
RUN chown -R "${cnb_uid}:${cnb_gid}" "/kaniko"

WORKDIR /layers

USER ${cnb_uid}:${cnb_gid}
