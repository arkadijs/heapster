#!/bin/bash

set -e

KUBE_ARGS=""

if [ ! -z $KUBERNETES_SERVICE_HOST ]; then
    echo "Detected Kube specific args. Starting in Kube mode."
    KUBE_ARGS="--kubernetes_master $KUBERNETES_SERVICE_HOST:$KUBERNETES_SERVICE_PORT"
fi

# Check if InfluxDB service is running
if [ ! -z $INFLUX_MASTER_SERVICE_PORT ]; then
# TODO(vishh): add support for passing in user name and password.
    /usr/bin/heapster $KUBE_ARGS -logtostderr -sink influxdb -influxdb_host "${INFLUX_MASTER_SERVICE_HOST}:${INFLUX_MASTER_SERVICE_PORT}"
elif [ ! -z $INFLUXDB_HOST ]; then
    /usr/bin/heapster $KUBE_ARGS -logtostderr -sink influxdb -influxdb_host ${INFLUXDB_HOST}
else
    /usr/bin/heapster $KUBE_ARGS -logtostderr
fi
