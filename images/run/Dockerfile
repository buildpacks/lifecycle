ARG base=packs/base

FROM ${base}
ARG stack_id="io.buildpacks.stacks.bionic"

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=1s --start-period=60s --retries=1 \
  CMD wget --quiet --tries=1 --spider http://localhost:8080/ || exit 1

USER pack

LABEL io.buildpacks.stack.id="${stack_id}"
ENV PACK_STACK_ID="${stack_id}"
ENV PACK_PROCESS_TYPE web

ENTRYPOINT ["/lifecycle/launcher"]
