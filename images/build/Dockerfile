ARG base=packs/base

FROM $base
ARG samples_tgz=https://api.github.com/repos/buildpack/samples/tarball/master

RUN mkdir /tmp/samples /buildpacks && \
  wget -q -O - "$samples_tgz" | tar -xzf - -C /tmp/samples --strip-components=1 --wildcards 'buildpack-samples-*' && \
  for buildpack in nodejs java; do \
    id=$(cat "/tmp/samples/${buildpack}-buildpack/buildpack.toml" | yj -t | jq -r .id) && \
    version=$(cat "/tmp/samples/${buildpack}-buildpack/buildpack.toml" | yj -t | jq -r .version) && \
    mkdir -p "/buildpacks/$id" && \
    mv "/tmp/samples/${buildpack}-buildpack" "/buildpacks/$id/$version" && \
    ln -s "$version" "/buildpacks/$id/latest" && \
    echo "[[groups]]\nbuildpacks = [ { id = \"sh.packs.samples.buildpack.${buildpack}\", version = \"latest\" } ]" > /buildpacks/order.toml; \
  done && \
  rm -rf /tmp/samples

RUN mkdir -p /workspace/app /workspace/config /cache /platform/env
RUN chown -R pack:pack /workspace /cache

USER pack

ENV PACK_BUILDPACKS_DIR /buildpacks
ENV PACK_ORDER_PATH /buildpacks/order.toml

ENV PACK_GROUP_PATH ./group.toml
ENV PACK_PLAN_PATH ./plan.toml

ENV PACK_STACK_ID io.buildpacks.stacks.bionic
ENV PACK_METADATA_PATH /workspace/config/metadata.toml
