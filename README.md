# Lifecycle

A reference implementation of Buildpack API v3.

## Commands

* `detector` - chooses buildpacks (via `/bin/detect`)
* `analyzer` - retrieves layer metadata from the previous build
* `builder` -  executes buildpacks (via `/bin/build`)
* `exporter` - remotely patches image with new layers (via rebase & append)
* `launcher` - invokes process

## Status

Only `detector` and `builder` are currently implemented.

`analyzer` and `exporter` are partially implemented in [packs](https://github.com/sclevine/packs).