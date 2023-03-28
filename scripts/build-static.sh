#!/bin/sh

SNAPSHOTTER_SOURCE="/build/coriolis-snapshot-agent"

cd $SNAPSHOTTER_SOURCE/cmd/coriolis-snapshot-agent
git config --global --add safe.directory $SNAPSHOTTER_SOURCE
go build -buildvcs=false -o $SNAPSHOTTER_SOURCE/coriolis-snapshot-agent -ldflags "-linkmode external -extldflags '-static' -s -w -X main.Version=$(git describe --tags)" .
