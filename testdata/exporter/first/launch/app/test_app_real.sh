#!/bin/sh

source ./layer1/file-from-layer-1
source /launch/buildpack.id/layer2/file-from-layer-2

cat subdir_symlink/myfile.txt

echo "Arg1 is '$1'"

echo "PATH: $PATH"
