ARG base=packs/base

FROM $base

RUN mkdir -p /workspace/app /workspace/config /cache /platform/env
RUN chown -R pack:pack /workspace /cache

USER pack

ENV PACK_BUILDPACKS_DIR /buildpacks
ENV PACK_ORDER_PATH /buildpacks/order.toml

ENV PACK_GROUP_PATH ./group.toml
ENV PACK_PLAN_PATH ./plan.toml

ENV PACK_STACK_ID io.buildpacks.stacks.bionic
