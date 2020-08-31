FROM ubuntu:bionic

RUN apt-get update && apt-get install -y ca-certificates

COPY container /

RUN chown -R 2222:3333 /layers
RUN chown -R 2222:3333 /cnb
RUN chown -R 2222:3333 /workspace

# create the snapshot fixture here so it has the right owner
RUN echo "hello" > file-to-snapshot && \
    tar -czvf example_stack.tgz file-to-snapshot && \
    mv example_stack.tgz /layers/ && \
    rm file-to-snapshot

WORKDIR /layers

ENV CNB_USER_ID=2222

ENV CNB_GROUP_ID=3333
