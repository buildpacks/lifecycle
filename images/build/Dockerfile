ARG base=packs/base

FROM $base
ARG stack_id="io.buildpacks.stacks.bionic"

RUN mkdir -p /workspace/app /workspace/config /cache /platform/env
RUN chown -R pack:pack /workspace /cache

USER pack
LABEL io.buildpacks.stack.id=${stack_id}
ENV PACK_STACK_ID=${stack_id}
