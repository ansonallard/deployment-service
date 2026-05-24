#!/bin/bash

# Mechanism to deploy latest version of service

SERVICE_NAME="deployment-service"
git fetch
tag=$(git --no-pager tag --sort=-creatordate | head -1)
git checkout $tag
go build -o $SERVICE_NAME ./cmd/$SERVICE_NAME
sudo systemctl restart deployment-service