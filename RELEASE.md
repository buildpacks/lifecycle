## Release Finalization

To cut a release:
1. Ensure the relevant spec APIs have been released
1. Ensure any [migration guides](https://github.com/buildpacks/docs/tree/main/content/docs/reference/spec/migration) have been created.
1. If applicable, ensure the README is updated with the latest supported apis (example PR: https://github.com/buildpacks/lifecycle/pull/550)
1. Create a release branch in the format `release/0.99.0`. New commits to this branch will trigger the `build` workflow and produce a lifecycle image: `buildpacksio/lifecycle:<commit sha>`.
1. When ready to cut the release, manually trigger the `draft-release` workflow: Actions -> draft-release -> Run workflow -> Use workflow from branch: `release/0.99.0`. This will create a draft release on GitHub using the artifacts from the `build` workflow run for the latest commit on the release branch.
1. Edit the release notes as necessary
1. Perform any manual validation of the artifacts
```
LIFECYCLE_PATH="/path/to/created/lifecycle-v0.99.0+linux.x86-64.tgz" make acceptance
```
1. When ready to publish the release, edit the release page and click "Publish release". This will trigger the `post-release` workflow that will re-tag the lifecycle image from `buildpacksio/lifecycle:<commit sha>` to `buildpacksio/lifecycle:0.99.0`.
1. Once released, update the `main` branch to remove the pre-release note in [README.md](https://github.com/buildpacks/lifecycle/blob/main/README.md)
