cd $LIFECYCLE_REPO_PATH

echo ">>>>>>>>>> Building lifecycle..."

docker image rm test-builder --force

make clean build-linux-amd64

cd out/linux-amd64

cat << EOF > Dockerfile
FROM cnbs/sample-builder:bionic
COPY ./lifecycle /cnb/lifecycle
EOF

docker build -t test-builder .

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
  test-builder \
  /cnb/lifecycle/detector -order /layers/order.toml -log-level debug

echo ">>>>>>>>>> Running build for extensions..."

docker run \
  -v $PWD/workspace/:/workspace \
  -v $PWD/layers/:/layers \
  -v $PWD/platform/:/platform \
  -v $PWD/cnb/ext/:/cnb/ext \
  -v $PWD/cnb/buildpacks/:/cnb/buildpacks \
  test-builder \
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
  test-builder \
  /cnb/lifecycle/extender kaniko build ubuntu:bionic
  #              args:    <kaniko|buildah> <build|run> <base-image>

echo ">>>>>>>>>> Running extend on run image..."

docker run \
  -v $PWD/workspace/:/workspace \
  -v $PWD/kaniko-run/:/kaniko \
  -v $PWD/layers-run/:/layers \
  -v $PWD/platform/:/platform \
  -v $PWD/cnb/ext/:/cnb/ext \
  -v $PWD/cnb/buildpacks/:/cnb/buildpacks \
  -u root \
  test-builder \
  /cnb/lifecycle/extender kaniko run ubuntu:bionic
  #              args:    <kaniko|buildah> <build|run> <base-image>

echo ">>>>>>>>>> Exporting final app image..."

docker run \
  -v $PWD/workspace/:/workspace \
  -v $PWD/layers-run/:/layers-run \
  -v $PWD/layers/:/layers \
  -v $PWD/platform/:/platform \
  -v $PWD/cnb/ext/:/cnb/ext \
  -v $PWD/cnb/buildpacks/:/cnb/buildpacks \
  -u root \
  test-builder \
  /cnb/lifecycle/exporter -run-image ubuntu:bionic -log-level debug $IMAGE_TAG

echo ">>>>>>>>>> Validate app image..."
# TODO: this fails because curl is not on there
docker run --rm --entrypoint bash -it $IMAGE_TAG -- curl google.com
