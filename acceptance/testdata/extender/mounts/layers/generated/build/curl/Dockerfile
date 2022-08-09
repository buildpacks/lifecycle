ARG base_image
FROM ${base_image}

ARG build_id=0
RUN echo ${build_id}

USER root
RUN apt-get update && apt-get install -y curl=7.58.0-2ubuntu3.19
