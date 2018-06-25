# Packs

[![Build Status](https://travis-ci.org/buildpack/packs.svg?branch=master)](https://travis-ci.org/buildpack/packs)
[![GoDoc](https://godoc.org/github.com/buildpack/packs?status.svg)](https://godoc.org/github.com/buildpack/packs)

This repo contains Packs: buildpack cloud builders for the Cloud Foundry and Heroku buildpacks.

[These cloud builders are available on Docker Hub.](https://hub.docker.com/u/packs/)

## Quick Start: Cloud Foundry Packs

Build:
```bash
docker run --rm -v "$(pwd)/app:/workspace" -v "$(pwd)/out:/out" packs/cf:build
```

Run:
```bash
docker run --rm -P \
    -v "$(pwd)/out:/workspace" \
    packs/cf:run -droplet droplet.tgz -metadata result.json
```

Export to Docker daemon:
```bash
docker run --rm \
    -v "$(pwd)/out:/workspace" \
    -v "/var/run/docker.sock:/var/run/docker.sock" \
    packs/cf:export -daemon -droplet droplet.tgz -metadata result.json my-image
```

Export to Docker registry:
```bash
docker run --rm \
    -v "$(pwd)/out:/workspace" \
    -v "$HOME/.docker/config.json:/root/.docker/config.json" \
    packs/cf:export -droplet droplet.tgz -metadata result.json my-image
```