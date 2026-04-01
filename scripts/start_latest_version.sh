#!/bin/bash

# Mechanism to deploy latest version of service

git fetch
tag=$(git --no-pager tag --sort=-creatordate | head -1)
git checkout $tag
go build .
sudo systemctl restart deployment-service