#!/bin/sh -xe

docker build -t kubernetes/heapster_influxdb .
t=docker-registry.r53.acp.io:5000/kubernetes/heapster_influxdb
docker tag -f kubernetes/heapster_influxdb $t:1
docker push $t
