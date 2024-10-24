# Release Finalization

## Types of releases

#### New minor
* For newly supported Platform or Buildpack API versions, or breaking changes (e.g., API deprecations).

#### Pre-release aka release candidate
* Ideally we should ship a pre-release (waiting a few days for folks to try it out) before we ship a new minor.
* We typically don't ship pre-releases for patches or backports.

#### New patch
* For go version updates, CVE fixes / dependency bumps, bug fixes, etc.
* Review the latest commits on `main` to determine if any are unacceptable for a patch - if there are commits that should be excluded, branch off the latest tag for the current minor and cherry-pick commits over.

#### Backport
* New patch for an old minor. Typically, to help folks out who haven't yet upgraded from [unsupported APIs](https://github.com/buildpacks/rfcs/blob/main/text/0110-deprecate-apis.md).
* For go version updates, CVE fixes / dependency bumps, bug fixes, etc.
* Branch off the latest tag for the desired minor.

## Release Finalization Steps

### Step 1 - Prepare

Determine the type of release ([new minor](#new-minor), [pre-release](#pre-release-aka-release-candidate), [new patch](#new-patch), or [backport](#backport)) and prepare the branch accordingly.

**To prepare the release branch:**
1. Check open PRs for any dependabot updates that should be merged.
1. Create a release branch in the format `release/0.99.0-rc.1` (for pre-releases) or `release/0.99.0` (for final releases).
   * New commits to this branch will trigger the `build` workflow and produce a lifecycle image: `buildpacksio/lifecycle:<commit sha>`.
1. If applicable, ensure the README is updated with the latest supported apis (example PR: https://github.com/buildpacks/lifecycle/pull/550).
   * For final releases (not pre-releases), remove the pre-release note (`*`) for the latest apis.

**For final releases (not pre-releases):**
1. Ensure the relevant spec APIs have been released.
1. Ensure the `lifecycle/0.99.0` milestone on the [docs repo](https://github.com/buildpacks/docs/blob/main/RELEASE.md#lump-changes) is complete, such that every new feature in the lifecycle is fully explained in the `release/lifecycle/0.99` branch on the docs repo, and [migration guides](https://github.com/buildpacks/docs/tree/main/content/docs/reference/spec/migration) (if relevant) are included.

### Step 2 - Publish the Release

1. Manually trigger the `draft-release` workflow: Actions -> draft-release -> Run workflow -> Use workflow from branch: `release/<release version>`. This will create a draft release on GitHub using the artifacts from the `build` workflow run for the latest commit on the release branch.
1. Edit the release notes as necessary.
1. Perform any manual validation of the artifacts as necessary (usually none).
1. Edit the release page and click "Publish release".
   * This will trigger the `post-release` workflow that will re-tag the lifecycle image from `buildpacksio/lifecycle:<commit sha>` to `buildpacksio/lifecycle:<release version>`.
     * For final releases ONLY, this will also re-tag the lifecycle image from `buildpacksio/lifecycle:<commit sha>` to `buildpacksio/lifecycle:latest`.

### Step 3 - Follow-up

**For pre-releases:**
* Ask the relevant teams to try out the pre-released artifacts.

**For final releases:**
* Update the `main` branch to remove the pre-release note in [README.md](https://github.com/buildpacks/lifecycle/blob/main/README.md) and/or merge `release/0.99.0` into `main`.
* Ask the learning team to merge the `release/lifecycle/0.99` branch into `main` on the docs repo.

## Go version updates

Go version updates should be released as a [new minor](#new-minor) or [new patch](#new-patch) release.

### New Patch

If the go patch is in [actions/go-versions](https://github.com/actions/go-versions/pulls?q=is%3Apr+is%3Aclosed) then CI should pull it in automatically without any action needed.
We simply need to create the release branch and let the pipeline run.

### New Minor

We typically do this when the existing patch version exceeds 6 - e.g., `1.22.6`. This means we have about 6 months to upgrade before the current minor becomes unsupported due to the introduction of the new n+2 minor.

#### Steps
1. Update go.mod
1. Search for the old `major.minor`, there are a few files that need to be updated (example PR: https://github.com/buildpacks/lifecycle/pull/1405/files)
1. Update the linter to a version that supports the current `major.minor`
1. Fix any lint errors as necessary
