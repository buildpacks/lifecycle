ARG base_image
FROM ${base_image}

USER root
RUN apt-get update && apt-get install -y curl

ARG build_id=0
RUN echo ${build_id}

LABEL io.buildpacks.rebasable=true
# curl is not really rebasable, but we set the label here to test that the label in the extended image is set correctly
