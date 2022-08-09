ARG base_image
FROM ${base_image}

ARG build_id=0
RUN echo ${build_id}

USER root
RUN apt-get update && apt-get install -y tree
