ARG base_image
FROM ${base_image}

USER root
RUN apt-get update && apt-get install -y curl

ARG build_id=0
RUN echo ${build_id}

# This should not create a layer as the path should be ignored by kaniko
RUN echo "some-content" > /workspace/some-file
