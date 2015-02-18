#!/bin/sh -xe

docker build -t kubernetes/heapster_grafana .
t=docker-registry.r53.acp.io:5000/kubernetes/heapster_grafana:5
docker tag -f kubernetes/heapster_grafana $t
docker push $t
