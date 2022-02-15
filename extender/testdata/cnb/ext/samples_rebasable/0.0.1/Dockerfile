ARG base_image
FROM ${base_image}
ARG build_id=0

LABEL io.buildpacks.rebasable=true

# this should not error
RUN ls /file-from-first-extension.txt

# TODO: replace with real package
# RUN curl -L https://example.com/mypkg-install | sh # installs to /opt
