#!/bin/bash

# $1 - registry repo name
# $2 - path to cosign public key

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

echo "Use own registry account (assumes DOCKER_PASSWORD and DOCKER_USERNAME have been added to GitHub secrets, if not using ghcr.io)"
sed -i '' "s/buildpacksio\/lifecycle/$registry\/$username\/lifecycle/g" .github/workflows/*.yml

if [[ $registry != "index.docker.io" ]]; then
  echo "Update login action to specify the login server"
  sed -i '' "s/username: \${{ secrets.DOCKER_USERNAME }}/login-server: $registry\n          username: $username/g" .github/workflows/*.yml
fi

# If using ghcr.io, we don't need to set the DOCKER_* secrets. Update the login action to use GitHub token instead.
if [[ $registry == *"ghcr.io"* ]]; then
  echo "Update login action to use GitHub token for ghcr.io"
  sed -i '' "s/secrets.DOCKER_PASSWORD/secrets.GITHUB_TOKEN/g" .github/workflows/*.yml
fi

echo "Use public key from fork (assumes private key and passphrase have been added to GitHub secrets)"
cp $2 cosign.pub

echo "Skip tests to make things faster"
sed -i '' "s/make test/echo test/g" .github/workflows/*.yml
sed -i '' "s/make acceptance/echo acceptance/g" .github/workflows/*.yml
