#!/bin/sh

source /launch/buildpack.id/layer1/file-from-layer-1
source /launch/buildpack.id/layer2/file-from-layer-2

echo "Arg1 is '$1'"

echo "PATH: $PATH"
