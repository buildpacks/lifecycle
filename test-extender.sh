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
