#!/bin/bash

set -e

latest_tag=$(git describe --tags --abbrev=0 2>/dev/null || echo "v9999.9999.9999")
latest_ver=${latest_tag#v}

# find the version number in ../../v3/core/grokker.go
grokker_version=$(perl -ne 'print $1 if /Version\s*=\s*"(\S+)"/'  ../../v3/core/grokker.go)

# ensure grokker_version is newer than latest_ver
echo "Latest tagged version: $latest_ver"
echo "Grokker version: $grokker_version"

# convert both version numbers to integers for comparison
semver_to_int() {
	parts=(${1//./ })
	local major=${parts[0]:-0}
	local minor=${parts[1]:-0}
	local patch=${parts[2]:-0}
	echo $((major * 1000000 + minor * 1000 + patch))
}

int_latest=$(semver_to_int "$latest_ver")
int_grokker=$(semver_to_int "$grokker_version")
echo "Latest tagged as int: $int_latest"
echo "Grokker version as int: $int_grokker"

if [ "$int_grokker" -le "$int_latest" ]; then
	echo "Error: grokker.go:Version $grokker_version is not greater than latest tag $latest_tag"
	exit 1
fi

if ! git diff --quiet --exit-code 
then
  echo "Working directory is dirty. Please commit changes before installing."
  exit 1
fi

git tag "v$grokker_version" 
echo "Tagged new version: v$grokker_version"
