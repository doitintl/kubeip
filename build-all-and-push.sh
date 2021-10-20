#!/usr/bin/env bash
export DOCKER_BUILDKIT=1
# BUILD
echo "Building"
make
# Make Image
echo "Creating Image"
make image
# Re-tagging docker file
echo "Tagging"
docker tag kubeip:latest ${REGISTRY}/kubeip:latest
# Pushing image
echo "Pushing Image"
gcloud docker -- push ${REGISTRY}kubeip:latest
