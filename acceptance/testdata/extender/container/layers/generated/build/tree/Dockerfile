ARG base_image
FROM ${base_image}

USER root
RUN apt-get update && apt-get install -y tree

ENV CNB_STACK_ID=stack-id-from-ext-tree

ARG build_id=0
RUN echo ${build_id}
