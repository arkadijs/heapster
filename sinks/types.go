package sinks

import (
	"flag"
	"fmt"

	"github.com/GoogleCloudPlatform/heapster/sources"
)

var argSink = flag.String("sink", "influxdb", "Backend storage, currently only `influxdb`.")

type Sink interface {
	StoreData(data sources.Data) error
}

const (
	statsTable            = "stats"
	specTable             = "spec"
	machineTable          = "machine"
	fsTable               = "filesystems"
	colTimestamp          = "time"
	colPodName            = "pod"
	colPodStatus          = "pod_status"
	colPodIP              = "pod_ip"
	colLabels             = "labels"
	colHostName           = "hostname"
	colContainerName      = "container_name"
	colCpuCumulativeUsage = "cpu_cumulative_usage"
	colCpuInstantUsage    = "cpu_instant_usage"
	colMemoryUsage        = "memory_usage"
	colMemoryWorkingSet   = "memory_working_set"
	colMemoryPgFaults     = "page_faults"
	colRxBytes            = "rx_bytes"
	colRxErrors           = "rx_errors"
	colTxBytes            = "tx_bytes"
	colTxErrors           = "tx_errors"
	colDiskIoServiceBytes = "diskio_service_bytes"
	colDiskIoServiced     = "diskio_serviced"
	colDiskIoQueued       = "diskio_queued"
	colDiskIoSectors      = "diskio_sectors"
	colDiskIoServiceTime  = "diskio_service_time"
	colDiskIoWaitTime     = "diskio_wait_time"
	colDiskIoMerged       = "diskio_merged"
	colDiskIoTime         = "diskio_time"
	colFsDevice           = "fs_device"
	colFsCapacity         = "fs_capacity"
	colFsUsage            = "fs_usage"
	colFsFreePercent      = "fs_free_percent"
	colFsIoTime           = "fs_iotime"
	colFsIoTimeWeighted   = "fs_iotime_weighted"
)

func NewSink() (Sink, error) {
	switch *argSink {
	case "influxdb":
		return NewInfluxdbSink()
	default:
		return nil, fmt.Errorf("Invalid sink specified - %s", *argSink)
	}
}
