#!/bin/sh -xe

godep go build -a github.com/GoogleCloudPlatform/heapster
strip heapster

docker build -t kubernetes/heapster .
t=docker-registry.r53.acp.io:5000/kubernetes/heapster:4
docker tag -f kubernetes/heapster $t
docker push $t
