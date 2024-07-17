#!/bin/bash

# $1 - registry repo name
# $2 - enable tests

echo "Parse registry: $1"
firstPart=$(echo $1 | cut -d/ -f1)
secondPart=$(echo $1 | cut -d/ -f2)
thirdPart=$(echo $1 | cut -d/ -f3)

registry=""
username=""
reponame=""
if [[ -z $thirdPart ]]; then # assume Docker Hub
  registry="index.docker.io"
  username=$firstPart
  reponame=$secondPart
else
  registry=$firstPart
  username=$secondPart
  reponame=$thirdPart
fi

echo "Using registry $registry and username $username"
if [[ $reponame != "lifecycle" ]]; then
  echo "Repo name must be 'lifecycle'"
  exit 1
fi

echo "Using own registry account (assumes DOCKER_PASSWORD and DOCKER_USERNAME have been added to GitHub secrets, if not using ghcr.io)"
sed -i '' "s/buildpacksio\/lifecycle/$registry\/$username\/lifecycle/g" .github/workflows/*.yml

if [[ $registry != "index.docker.io" ]]; then
  echo "Updating login action to specify the login server"
  sed -i '' "s/username: \${{ secrets.DOCKER_USERNAME }}/login-server: $registry\n          username: $username/g" .github/workflows/*.yml
fi

if [[ $registry == *"ghcr.io"* ]]; then
  # If using ghcr.io, we don't need to set the DOCKER_* secrets. Update the login action to use GitHub token instead.
  echo "Updating login action to use GitHub token for ghcr.io"
  sed -i '' "s/secrets.DOCKER_PASSWORD/secrets.GITHUB_TOKEN/g" .github/workflows/*.yml

  echo "Adding workflow permissions to push images to ghcr.io"
  LF=$'\n'
  sed -i '' "/      id-token: write/ a\\
      contents: read\\
      packages: write\\
      attestations: write${LF}" .github/workflows/build.yml
  sed -i '' "/      id-token: write/ a\\
      contents: read\\
      packages: write\\
      attestations: write${LF}" .github/workflows/post-release.yml
  LF=""
fi

echo "Removing arm tests (these require a self-hosted runner)"
sed -i '' "/test-linux-arm64:/,+14d" .github/workflows/build.yml
sed -i '' "/test-linux-arm64/d" .github/workflows/build.yml

if [[ -z $2 ]]; then
  echo "Removing all tests to make things faster"
  sed -i '' "s/make test/echo test/g" .github/workflows/*.yml
  sed -i '' "s/make acceptance/echo acceptance/g" .github/workflows/*.yml
  echo "$(sed '/pack-acceptance/,$d' .github/workflows/build.yml)" > .github/workflows/build.yml
  echo "Removing Codecov"
  sed -i '' "/- name: Upload coverage to Codecov/,+7d" .github/workflows/build.yml
  sed -i '' "/- name: Prepare Codecov/,+6d" .github/workflows/build.yml
else
  echo "Retaining tests"
fi

