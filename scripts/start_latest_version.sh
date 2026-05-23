#!/bin/bash

# Mechanism to deploy latest version of service

SERVICE_NAME="deployment-service"
git fetch
tag=$(git --no-pager tag --sort=-creatordate | head -1)
git checkout $tag
MAIN_DIR=$(find "$PWD" -type d -path "*/cmd/$SERVICE_NAME")
go build -o $SERVICE_NAME $MAIN_DIR
sudo systemctl restart deployment-service