FROM ubuntu:bionic

COPY container /

ENV PATH="/cnb/process:/cnb/lifecycle:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"

ENTRYPOINT ["/cnb/lifecycle/launcher"]
