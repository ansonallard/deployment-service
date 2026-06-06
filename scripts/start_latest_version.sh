#!/bin/bash

# Mechanism to deploy latest version of service
SERVICE_NAME="deployment-service"
git fetch
current_tag=$(git describe --tags --exact-match 2>/dev/null)
tag=$(git --no-pager tag --sort=-creatordate | head -1)

if [[ $tag == $current_tag ]]; then
    echo "No changes, terminating" >&2
    exit 0
fi

git checkout "$tag"
go build -o "$SERVICE_NAME" ./cmd/"$SERVICE_NAME"
sudo systemctl restart deployment-service