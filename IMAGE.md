# Quick reference

This image is maintained by the [Cloud Native Buildpacks project](https://buildpacks.io/). The maintainers can be contacted via the [Cloud Native Buildpacks Slack](https://slack.buildpacks.io/), or by opening an issue on the `buildpacks/lifecycle` [GitHub repo](https://github.com/buildpacks/lifecycle).

# Supported tags

Supported tags are semver-versioned manifest lists - e.g., `0.12.0` or `0.12.0-rc.1`, pointing to one of the following os/architectures:
* `linux/amd64`
* `linux/arm64`
* `windows/amd64`

# About this image

Images are built in [GitHub actions](https://github.com/buildpacks/lifecycle/actions) and signed with [`cosign`](https://github.com/sigstore/cosign). To verify:
* Locate the public key `lifecycle-v<tag>-cosign.pub` on the [releases page](https://github.com/buildpacks/lifecycle/releases)
* Run:
```
cosign verify -key ~/lifecycle-v<tag>-cosign.pub buildpacksio/lifecycle:<tag>
```

A CycloneDX SBOM is "attached" to the image and signed with [`cosign`](https://github.com/sigstore/cosign). To verify:
* Locate the public key `lifecycle-v<tag>-cosign.pub` on the [releases page](https://github.com/buildpacks/lifecycle/releases)
* Run:
```
digest=$(cosign verify -key ~/lifecycle-v<tag>-cosign.pub buildpacksio/lifecycle:<tag> | jq -r .critical.image.\"docker-manifest-digest\" | head -n 1)
digest=${digest#"sha256:"}
cosign verify -key ~/lifecycle-v<tag>-cosign.pub buildpacksio/lifecycle:sha256-${digest}.sbom
cosign download sbom buildpacksio/lifecycle:<tag>
```

# Using this image

With [pack](https://github.com/buildpack/pack):
* `pack build <target> --lifecycle-image buildpacksio/lifecycle:<tag>`

With [tekton](https://github.com/tektoncd/catalog/tree/main/task/buildpacks-phases/0.2):
* Provide as param `LIFECYCLE_IMAGE` in taskrun
