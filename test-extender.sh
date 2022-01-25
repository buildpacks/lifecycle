set -e

cd $LIFECYCLE_REPO_PATH

echo ">>>>>>>>>> Building lifecycle..."

docker image rm $REGISTRY_HOST/test-builder --force
# remove the $REGISTRY_HOST/extended/runimage from any previous run
docker image rm $REGISTRY_HOST/extended/runimage --force

make clean build-linux-amd64

cd out/linux-amd64

cat << EOF > Dockerfile
FROM cnbs/sample-builder:bionic
COPY ./lifecycle /cnb/lifecycle
EOF

docker build -t $REGISTRY_HOST/test-builder .
docker push $REGISTRY_HOST/test-builder

cd $SAMPLES_REPO_PATH

rm -rf $SAMPLES_REPO_PATH/kaniko
mkdir -p $SAMPLES_REPO_PATH/kaniko
rm -rf $SAMPLES_REPO_PATH/layers/kaniko
mkdir -p $SAMPLES_REPO_PATH/layers/kaniko
rm -rf $SAMPLES_REPO_PATH/kaniko-run
mkdir -p $SAMPLES_REPO_PATH/kaniko-run
rm -rf $SAMPLES_REPO_PATH/layers-run/kaniko
mkdir -p $SAMPLES_REPO_PATH/layers-run/kaniko

echo ">>>>>>>>>> Running detect..."

docker run \
  -v $PWD/workspace/:/workspace \
  -v $PWD/layers/:/layers \
  -v $PWD/platform/:/platform \
  -v $PWD/cnb/ext/:/cnb/ext \
  -v $PWD/cnb/buildpacks/:/cnb/buildpacks \
  $REGISTRY_HOST/test-builder \
  /cnb/lifecycle/detector -order /layers/order.toml -log-level debug

echo ">>>>>>>>>> Running build for extensions..."

docker run \
  -v $PWD/workspace/:/workspace \
  -v $PWD/layers/:/layers \
  -v $PWD/platform/:/platform \
  -v $PWD/cnb/ext/:/cnb/ext \
  -v $PWD/cnb/buildpacks/:/cnb/buildpacks \
  $REGISTRY_HOST/test-builder \
  /cnb/lifecycle/builder -use-extensions -log-level debug

echo ">>>>>>>>>> Copy build extension layers for run extension..."

# this needs to come along for the ride for extending run image
cp -R $PWD/layers/* $PWD/layers-run

echo ">>>>>>>>>> Running extend on build image followed by build for buildpacks..."

docker run \
  -v $PWD/workspace/:/workspace \
  -v $PWD/kaniko/:/kaniko \
  -v $PWD/layers/:/layers \
  -v $PWD/platform/:/platform \
  -v $PWD/cnb/ext/:/cnb/ext \
  -v $PWD/cnb/buildpacks/:/cnb/buildpacks \
  -u root \
  $REGISTRY_HOST/test-builder \
  /cnb/lifecycle/extender kaniko build "$REGISTRY_HOST/test-builder"
  #              args:    <kaniko|buildah> <build|run> <base-image>

echo ">>>>>>>>>> Running extend on run image..."
 # TODO: we should probably not have to mount /cnb/ext? Can we copy the static Dockerfiles to layers?
docker run \
  -v $PWD/workspace/:/workspace \
  -v $PWD/kaniko-run/:/kaniko \
  -v $PWD/layers-run/:/layers \
  -v $PWD/cnb/ext/:/cnb/ext \
  -u root \
  -e REGISTRY_HOST=$REGISTRY_HOST \
  $REGISTRY_HOST/test-builder \
  /cnb/lifecycle/extender kaniko run cnbs/sample-stack-run:bionic
  #              args:    <kaniko|buildah> <build|run> <base-image>

echo ">>>>>>>>>> Validate extended run image..."
docker load -i $PWD/layers-run/kaniko/new_base.tar
docker run --rm -it $REGISTRY_HOST/extended/runimage cat /opt/arg.txt
docker run --rm -it $REGISTRY_HOST/extended/runimage curl google.com

# echo ">>>>>>>>>> Exporting final app image..."

docker run \
  -v $PWD/workspace/:/workspace \
  -v $PWD/layers-run/:/layers-run \
  -v $PWD/layers/:/layers \
  -v $PWD/platform/:/platform \
  -v $PWD/cnb/ext/:/cnb/ext \
  -v $PWD/cnb/buildpacks/:/cnb/buildpacks \
  -u root \
  $REGISTRY_HOST/test-builder \
  /cnb/lifecycle/exporter -log-level debug -run-image $REGISTRY_HOST/extended/runimage $REGISTRY_HOST/appimage

# echo ">>>>>>>>>> Validate app image..."
docker pull $REGISTRY_HOST/appimage
docker run --rm --entrypoint cat -it $REGISTRY_HOST/appimage /opt/arg.txt
# # TODO: this fails because "error while loading shared libraries: libnghttp2.so.14: cannot open shared object file: No such file or directory"
docker run --rm --entrypoint curl -it $REGISTRY_HOST/appimage google.com
