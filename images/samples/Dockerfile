ARG base=packs/build

FROM $base
ARG samples_tgz=https://api.github.com/repos/buildpack/samples/tarball/master

USER root

RUN \
  mkdir /tmp/samples /buildpacks && \
  wget -q -O - "$samples_tgz" | tar -xzf - -C /tmp/samples --strip-components=1 --wildcards 'buildpack-samples-*' && \
  for buildpack in nodejs java; do \
    id=$(cat "/tmp/samples/${buildpack}-buildpack/buildpack.toml" | yj -t | jq -r .buildpack.id) && \
    version=$(cat "/tmp/samples/${buildpack}-buildpack/buildpack.toml" | yj -t | jq -r .buildpack.version) && \
    mkdir -p "/buildpacks/$id" && \
    mv "/tmp/samples/${buildpack}-buildpack" "/buildpacks/$id/$version" && \
    ln -s "$version" "/buildpacks/$id/latest" && \
    echo "[[groups]]\nbuildpacks = [ { id = \"${id}\", version = \"latest\" } ]" >> /buildpacks/order.toml; \
  done && \
  rm -rf /tmp/samples

USER pack
