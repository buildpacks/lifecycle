# Buildpack v3 reference implementation

## Usage

Create your `workspace` dir:

```sh-session
$ cd /tmp
$ mkdir workspace
$ cp -R /path/to/your/app workspace/app
```

Create a volume for the cache:

```
$ docker volume create --name packs_cache
```

Detect:

```
$ docker run --rm -v "$(pwd)/workspace:/workspace" packs/build /lifecycle/detector
```

Analyze:

```
$ docker run --rm -v "$(pwd)/workspace:/workspace" packs/build /lifecycle/analyzer
```

Build:

```
$ docker run --rm -v "$(pwd)/workspace:/workspace" -v "packs_cache:/cache" packs/build /lifecycle/builder
```

Run:

```
$ docker run --rm -P -v "$(pwd)/workspace:/workspace" packs/run
```

Export:

```
$ docker run --rm -v "$(pwd)/workspace:/workspace" \
  -e PACK_RUN_IMAGE="packs/run" -e PACK_LAUNCH_DIR="/workspace" \
  packs/util /lifecycle/exporter myimage
```

## Building the images

To build the packs/v3 images yourself, you'll need Stephen's™️ YAML to JSON converter:

```
$ go get github.com/sclevine/yj
$ cd $GOPATH/sclevine/yj
$ dep ensure
$ go install
```

Then build the images by running:

```
$ bin/build
```