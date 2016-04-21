#!/usr/bin/env bash

set -e

# install glide
# make sure that $GOPATH/bin is in your $PATH
if [ "$(which glide)" == "" ]; then
  go get github.com/Masterminds/glide
fi

# install dependencies
glide install

# for intellij, otherwise the imports are not working
cd vendor
ln -s . src