# Development

## Prerequisites

* [Git](https://git-scm.com/book/en/v2/Getting-Started-Installing-Git)
    * macOS: _(built-in)_
    * Windows:
        * `choco install git -y`
        * `git config --global core.autocrlf false`
* [Go](https://golang.org/doc/install)
    * macOS: `brew install go`
    * Windows: `choco install golang -y`
* [Docker](https://www.docker.com/products/docker-desktop)
* Make (and build tools)
    * macOS: `xcode-select --install`
    * Windows:
        * `choco install cygwin make -y`
        * `[Environment]::SetEnvironmentVariable("PATH", "C:\tools\cygwin\bin;$ENV:PATH", "MACHINE")`
        
### Caveats

* The acceptance tests require the docker daemon to be able to communicate with a local containerized insecure registry. On Docker Desktop 3.3.x, this may result in failures such as: `Expected nil: push response: : Get http://localhost:<port>/v2/: dial tcp [::1]:<port>: connect: connection refused`. To fix these failures, it may be necessary to add the following to the Docker Desktop Engine config:
    * macOS: Docker > Preferences > Docker Engine:
```
  "insecure-registries": {
    "<my-host-ip>/32"
  }
```


## Tasks

To test, build, and package binaries into an archive, simply run:

```bash
$ make all
```
This will create archives at `out/lifecycle-<LIFECYCLE_VERSION>+linux.x86-64.tgz` and `out/lifecycle-<LIFECYCLE_VERSION>+windows.x86-64.tgz`.

`LIFECYCLE_VERSION` defaults to the value returned by `git describe --tags` if not on a release branch (for more information about the release process, see [RELEASE](RELEASE.md)). It can be changed by prepending `LIFECYCLE_VERSION=<some version>` to the
`make` command. For example:

```bash
$ LIFECYCLE_VERSION=1.2.3 make all
```

Steps can also be run individually as shown below.

### Test

Formats, vets, and tests the code.

```bash
$ make test
```

### Build

Builds binaries to `out/linux/lifecycle/` and `out/windows/lifecycle/`.

```bash
$ make build
```

> To clean the `out/` directory, run `make clean`.

### Package

Creates archives at `out/lifecycle-<LIFECYCLE_VERSION>+linux.x86-64.tgz` and `out/lifecycle-<LIFECYCLE_VERSION>+windows.x86-64.tgz`, using the contents of the
`out/linux/lifecycle/` directory, for the given (or default) `LIFECYCLE_VERSION`.

```bash
$ make package
```
