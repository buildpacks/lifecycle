ARG base_image
FROM ${base_image}

USER root
RUN apt-get update && apt-get install -y curl
COPY build-file /

ARG build_id=0
RUN echo ${build_id}
