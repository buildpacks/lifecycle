ARG base_image
FROM ${base_image}

# TODO: why wasn't this needed before?
USER root

ARG some_arg
RUN echo ${some_arg} > /opt/arg.txt

ARG build_id=0
RUN echo ${build_id}

RUN apt-get update && apt-get install -y curl && rm -rf /var/lib/apt/lists/*
