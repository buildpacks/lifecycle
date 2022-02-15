set -e

echo ">>>>>>>>>> Preparing registry..."

if [ -z "$REGISTRY_HOST" ]; then
  REGISTRY_HOST="localhost:6000"
fi
echo "REGISTRY_HOST: $REGISTRY_HOST"

# Remove output images from daemon - note that they still exist in the local registry
docker image rm $REGISTRY_HOST/test-builder --force
docker image rm $REGISTRY_HOST/extended/buildimage --force # build image to extend
docker image rm $REGISTRY_HOST/extended/runimage --force   # run image to extend
docker image rm $REGISTRY_HOST/appimage --force

echo ">>>>>>>>>> Building lifecycle..."

if [ -z "$LIFECYCLE_REPO_PATH" ]; then
  LIFECYCLE_REPO_PATH=~/workspace/lifecycle
fi
cd $LIFECYCLE_REPO_PATH

make clean build-linux-amd64

echo ">>>>>>>>>> Building build base image..."

cd $LIFECYCLE_REPO_PATH/out/linux-amd64

cat <<EOF >Dockerfile
FROM cnbs/sample-builder:bionic
COPY ./lifecycle /cnb/lifecycle
EOF
docker build -t $REGISTRY_HOST/test-builder .
docker push $REGISTRY_HOST/test-builder

echo ">>>>>>>>>> Building extender minimal image..."

cat <<EOF >Dockerfile.extender
FROM gcr.io/distroless/static
COPY ./lifecycle /cnb/lifecycle
CMD /cnb/lifecycle/extender
EOF
docker build -f Dockerfile.extender -t $REGISTRY_HOST/extender .
docker push $REGISTRY_HOST/extender

echo ">>>>>>>>>> Preparing fixtures..."

if [ -z "$SAMPLES_REPO_PATH" ]; then
  SAMPLES_REPO_PATH=~/workspace/samples
fi
cd $SAMPLES_REPO_PATH

rm -rf $SAMPLES_REPO_PATH/kaniko
mkdir -p $SAMPLES_REPO_PATH/kaniko
rm -rf $SAMPLES_REPO_PATH/kaniko-run
mkdir -p $SAMPLES_REPO_PATH/kaniko-run

git status | grep "On branch dockerfiles-poc"

echo ">>>>>>>>>> Running detect..."

docker run \
  -v $PWD/cnb/buildpacks/:/cnb/buildpacks \
  -v $PWD/cnb/ext/:/cnb/ext \
  -v $PWD/layers/:/layers \
  -v $PWD/platform/:/platform \
  -v $PWD/workspace/:/workspace \
  $REGISTRY_HOST/test-builder \
  /cnb/lifecycle/detector -order /layers/order.toml -log-level debug

echo ">>>>>>>>>> Running build for extensions..."

docker run \
  -v $PWD/cnb/buildpacks/:/cnb/buildpacks \
  -v $PWD/cnb/ext/:/cnb/ext \
  -v $PWD/layers/:/layers \
  -v $PWD/platform/:/platform \
  -v $PWD/workspace/:/workspace \
  $REGISTRY_HOST/test-builder \
  /cnb/lifecycle/builder -use-extensions -log-level debug

# Copy output /layers/config/metadata.toml so that the run extender can use it
# (otherwise it will be overwritten when running build for buildpacks)
cp $PWD/layers/config/metadata.toml $PWD/layers/config/extend-metadata.toml

echo ">>>>>>>>>> Running extend on build image followed by build for buildpacks..."

docker run \
  -v $PWD/cnb/buildpacks/:/cnb/buildpacks \
  -v $PWD/cnb/ext/:/cnb/ext \
  -v $PWD/kaniko/:/kaniko \
  -v $PWD/layers/:/layers \
  -v $PWD/workspace/:/workspace \
  -e REGISTRY_HOST=$REGISTRY_HOST \
  -u root \
  --network host \
  $REGISTRY_HOST/extender \
  /cnb/lifecycle/extender \
  -app /workspace \
  -config /layers/config/metadata.toml \
  -kind build \
  -log-level debug \
  -work-dir /kaniko \
  "$REGISTRY_HOST/test-builder"

docker pull $REGISTRY_HOST/extended/buildimage

echo ">>>>>>>>>> Running extend on run image..."

docker run \
  -v $PWD/cnb/ext/:/cnb/ext \
  -v $PWD/kaniko-run/:/kaniko \
  -v $PWD/layers/:/layers \
  -v $PWD/workspace/:/workspace \
  -e REGISTRY_HOST=$REGISTRY_HOST \
  -u root \
  --network host \
  $REGISTRY_HOST/extender \
  /cnb/lifecycle/extender \
  -app /workspace \
  -config /layers/config/extend-metadata.toml \
  -kind run \
  -log-level debug \
  -work-dir /kaniko \
  cnbs/sample-stack-run:bionic

docker pull $REGISTRY_HOST/extended/runimage

echo ">>>>>>>>>> Exporting final app image..."

docker run \
  -v $PWD/cnb/buildpacks/:/cnb/buildpacks \
  -v $PWD/cnb/ext/:/cnb/ext \
  -v $PWD/layers/:/layers \
  -v $PWD/platform/:/platform \
  -v $PWD/workspace/:/workspace \
  -u root \
  --network host \
  $REGISTRY_HOST/test-builder \
  /cnb/lifecycle/exporter -log-level debug -run-image $REGISTRY_HOST/extended/runimage $REGISTRY_HOST/appimage

echo ">>>>>>>>>> Validating app image..."

docker pull $REGISTRY_HOST/appimage
docker run --rm --entrypoint cat -it $REGISTRY_HOST/appimage /opt/arg.txt
docker run --rm --entrypoint curl -it $REGISTRY_HOST/appimage google.com
