ARG base_image
FROM ${base_image}

USER root
RUN apt-get update && apt-get install -y tree
COPY shared-file /shared-run

ENV CNB_STACK_ID=stack-id-from-ext-tree

ARG build_id=0
RUN echo ${build_id}

# tree is not really rebasable, but we set the label here to test that the label in the extended image is set correctly
LABEL io.buildpacks.rebasable=true

ARG user_id
USER ${user_id}
