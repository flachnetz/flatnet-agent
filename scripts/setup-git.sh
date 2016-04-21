#!/usr/bin/env sh

set -e

cd $(dirname "$0")

SCRIPTPATH=$(pwd)

. ${SCRIPTPATH}/git.properties

git config --global user.email ${GIT_EMAIL}
git config --global user.name "${GIT_NAME}"
git remote add release ${GITHUB_RELEASE_REPO} || true
