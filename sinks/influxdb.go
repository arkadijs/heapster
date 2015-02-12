package sinks

import (
	"flag"
	"fmt"
	"strings"
	"time"

	"github.com/GoogleCloudPlatform/heapster/sources"
	"github.com/golang/glog"
	cadvisor "github.com/google/cadvisor/info"
	influxdb "github.com/influxdb/influxdb/client"
)

var (
	argBufferDuration = flag.Duration("influxdb_buffer_duration", 1*time.Second, "Time duration for which stats should be buffered in InfluxDB sink before being written as a single transaction")
	argDbUsername     = flag.String("influxdb_username", "root", "InfluxDB username")
	argDbPassword     = flag.String("influxdb_password", "root", "InfluxDB password")
	argDbHost         = flag.String("influxdb_host", "localhost:8086", "InfluxDB host:port")
	argDbName         = flag.String("influxdb_name", "k8s", "InfluxDB database name")
	argDropUnknown    = flag.Bool("influxdb_drop_unknown_queries", false, "Drop unknown InfluxDB continuous queries at start")
)

type InfluxdbSink struct {
	client         *influxdb.Client
	dbName         string
	bufferDuration time.Duration
	lastWrite      time.Time
	sink           chan *[]*influxdb.Series
}

func (self *InfluxdbSink) getDefaultSeriesData(pod *sources.Pod, hostname, containerName string, stat *cadvisor.ContainerStats) (columns []string, values []interface{}) {
	// Timestamp
	columns = append(columns, colTimestamp)
	values = append(values, stat.Timestamp.Unix())

	if pod != nil {
		// Pod name
		columns = append(columns, colPodName)
		values = append(values, pod.Name)

		// Pod Status
		columns = append(columns, colPodStatus)
		values = append(values, pod.Status)

		// Pod IP
		columns = append(columns, colPodIP)
		values = append(values, pod.PodIP)

		labels := []string{}
		for key, value := range pod.Labels {
			labels = append(labels, fmt.Sprintf("%s:%s", key, value))
		}
		columns = append(columns, colLabels)
		values = append(values, strings.Join(labels, ","))
	}

	// Hostname
	columns = append(columns, colHostName)
	values = append(values, hostname)

	// Container name
	columns = append(columns, colContainerName)
	values = append(values, containerName)

	return
}

func (self *InfluxdbSink) containerFsStatsToSeries(tableName, hostname, containerName string, spec cadvisor.ContainerSpec, stat *cadvisor.ContainerStats, pod *sources.Pod) (series []*influxdb.Series) {
	if len(stat.Filesystem) == 0 {
		return
	}
	for _, fsStat := range stat.Filesystem {
		columns, values := self.getDefaultSeriesData(pod, hostname, containerName, stat)

		columns = append(columns, colFsDevice)
		values = append(values, fsStat.Device)

		columns = append(columns, colFsCapacity)
		values = append(values, fsStat.Limit)

		columns = append(columns, colFsUsage)
		values = append(values, fsStat.Usage)

		if fsStat.Limit > 0 {
			columns = append(columns, colFsFreePercent)
			values = append(values, (fsStat.Limit-fsStat.Usage)*100/fsStat.Limit)
		}

		columns = append(columns, colFsIoTime)
		values = append(values, fsStat.IoTime)

		columns = append(columns, colFsIoTimeWeighted)
		values = append(values, fsStat.WeightedIoTime)

		series = append(series, self.newSeries(tableName, columns, values))
	}
	return series

}

func agg(stats *[]cadvisor.PerDiskStats, what string) (ret uint64) {
	for _, disk := range *stats {
		ret += disk.Stats[what]
	}
	return
}

func sum(stats *[]cadvisor.PerDiskStats) uint64 {
	return agg(stats, "Total")
}

func count(stats *[]cadvisor.PerDiskStats) uint64 {
	return agg(stats, "Count")
}

func (self *InfluxdbSink) containerStatsToValues(pod *sources.Pod, hostname, containerName string, spec cadvisor.ContainerSpec, stat *cadvisor.ContainerStats) (columns []string, values []interface{}) {
	columns, values = self.getDefaultSeriesData(pod, hostname, containerName, stat)
	if spec.HasCpu {
		// Cumulative Cpu Usage
		columns = append(columns, colCpuCumulativeUsage)
		values = append(values, stat.Cpu.Usage.Total)
	}

	if spec.HasMemory {
		// Memory Usage
		columns = append(columns, colMemoryUsage)
		values = append(values, stat.Memory.Usage)

		// Memory Page Faults
		columns = append(columns, colMemoryPgFaults)
		values = append(values, stat.Memory.ContainerData.Pgfault)

		// Working set size
		columns = append(columns, colMemoryWorkingSet)
		values = append(values, stat.Memory.WorkingSet)
	}

	// Optional: Network stats.
	if spec.HasNetwork {
		columns = append(columns, colRxBytes)
		values = append(values, stat.Network.RxBytes)

		columns = append(columns, colRxErrors)
		values = append(values, stat.Network.RxErrors)

		columns = append(columns, colTxBytes)
		values = append(values, stat.Network.TxBytes)

		columns = append(columns, colTxErrors)
		values = append(values, stat.Network.TxErrors)
	}

	// DiskIo stats.
	// TODO(vishh): Use spec.HasDiskIo once that is exported by cadvisor.
	columns = append(columns, colDiskIoServiceBytes)
	values = append(values, sum(&stat.DiskIo.IoServiceBytes))
	columns = append(columns, colDiskIoServiced)
	values = append(values, sum(&stat.DiskIo.IoServiced))
	columns = append(columns, colDiskIoQueued)
	values = append(values, sum(&stat.DiskIo.IoQueued))
	columns = append(columns, colDiskIoSectors)
	values = append(values, count(&stat.DiskIo.Sectors))
	columns = append(columns, colDiskIoServiceTime)
	values = append(values, sum(&stat.DiskIo.IoServiceTime))
	columns = append(columns, colDiskIoWaitTime)
	values = append(values, sum(&stat.DiskIo.IoWaitTime))
	columns = append(columns, colDiskIoMerged)
	values = append(values, sum(&stat.DiskIo.IoMerged))
	columns = append(columns, colDiskIoTime)
	values = append(values, count(&stat.DiskIo.IoTime))
	return
}

// Returns a new influxdb series.
func (self *InfluxdbSink) newSeries(tableName string, columns []string, points []interface{}) *influxdb.Series {
	out := &influxdb.Series{
		Name:    tableName,
		Columns: columns,
		// There's only one point for each stats
		Points: make([][]interface{}, 1),
	}
	out.Points[0] = points
	return out
}

func (self *InfluxdbSink) handlePods(pods []sources.Pod) *[]*influxdb.Series {
	series := make([]*influxdb.Series, 0)
	for _, pod := range pods {
		for _, container := range pod.Containers {
			for _, stat := range container.Stats {
				col, val := self.containerStatsToValues(&pod, pod.Hostname, container.Name, container.Spec, stat)
				series = append(series, self.newSeries(statsTable, col, val))
				series = append(series, self.containerFsStatsToSeries(statsTable, pod.Hostname, container.Name, container.Spec, stat, &pod)...)
			}
		}
	}
	return &series
}

func (self *InfluxdbSink) handleContainers(containers []sources.RawContainer, tableName string) *[]*influxdb.Series {
	series := make([]*influxdb.Series, 0)
	// TODO(vishh): Export spec into a separate table and update it whenever it changes.
	for _, container := range containers {
		for _, stat := range container.Stats {
			col, val := self.containerStatsToValues(nil, container.Hostname, container.Name, container.Spec, stat)
			series = append(series, self.newSeries(tableName, col, val))
			series = append(series, self.containerFsStatsToSeries(fsTable, container.Hostname, container.Name, container.Spec, stat, nil)...)
		}
	}
	return &series
}

func (self *InfluxdbSink) flusher() {
	buffer := make([]*influxdb.Series, 0)
	ticker := time.NewTicker(self.bufferDuration)
	defer ticker.Stop()
	for {
		select {
		case series := <-self.sink:
			buffer = append(buffer, *series...)

		case <-ticker.C:
			if len(buffer) > 0 {
				go func(series []*influxdb.Series) {
					glog.V(2).Info("starting data flush to InfluxDB")
					if err := self.client.WriteSeriesWithTimePrecision(series, influxdb.Second); err != nil {
						glog.Errorf("Failed to write stats to InfluxDB: %v", err)
					} else {
						glog.V(2).Info("flushed data to InfluxDB")
					}
				}(buffer)
				buffer = make([]*influxdb.Series, 0)
			}
		}
	}
}

func (self *InfluxdbSink) StoreData(_data sources.Data) error {
	if data, ok := _data.(sources.ContainerData); ok {
		self.sink <- self.handlePods(data.Pods)
		self.sink <- self.handleContainers(data.Containers, statsTable)
		self.sink <- self.handleContainers(data.Machine, machineTable)
		return nil
	} else {
		return fmt.Errorf("Requesting unrecognized type to be stored in InfluxDB")
	}
}

func NewInfluxdbSink() (Sink, error) {
	config := &influxdb.ClientConfig{
		Host:     *argDbHost,
		Username: *argDbUsername,
		Password: *argDbPassword,
		Database: *argDbName,
		IsSecure: false,
	}
	client, err := influxdb.NewClient(config)
	if err != nil {
		return nil, err
	}
	client.DisableCompression()
	createDatabase := true
	if databases, err := client.GetDatabaseList(); err == nil {
		for _, database := range databases {
			if database["name"] == *argDbName {
				createDatabase = false
				break
			}
		}
	}
	if createDatabase {
		if err := client.CreateDatabase(*argDbName); err != nil {
			glog.Infof("Database creation failed: %v", err)
			return nil, err
		}
	}
	flusherChannel := make(chan *[]*influxdb.Series, 10)
	sink := &InfluxdbSink{
		client:         client,
		dbName:         *argDbName,
		bufferDuration: *argBufferDuration,
		sink:           flusherChannel,
	}
	go sink.flusher()
	go recreateContinuousQueries(client)
	return sink, nil
}

func seriesName(query string) string {
	i := strings.LastIndex(query, " ")
	return query[i+1:]
}

func dropContinuousQuery(client *influxdb.Client, id int) error {
	_, err := client.Query(fmt.Sprintf("drop continuous query %d", id))
	if err != nil {
		glog.Errorf("Cannot drop InfluxDB continuous query `%d`: %v", id, err)
	}
	return err
}

func recreateContinuousQueries(client *influxdb.Client) {
	infra := "/^(deis-|registrator|skydns|cadvisor|monitoring-|docker-registry|development-)/"
	var _queries = []string{
		// the queries must be exactly the same as 'list continuous queries' formats them
		"select container_name,derivative(cpu_cumulative_usage) as cpu_usage from \"stats\" where container_name !~ %s group by time(10s),container_name,hostname into cpu_stats_apps",
		"select container_name,derivative(cpu_cumulative_usage) as cpu_usage from \"stats\" where container_name =~ %s group by time(10s),container_name,hostname into cpu_stats_infra",
		"select container_name,mean(memory_usage) as memory_usage from \"stats\" where container_name !~ %s group by time(10s),container_name,hostname into memory_stats_apps",
		"select container_name,mean(memory_usage) as memory_usage from \"stats\" where container_name =~ %s group by time(10s),container_name,hostname into memory_stats_infra",
		"select container_name,derivative(rx_bytes + tx_bytes) as net_io from \"stats\" where container_name !~ %s group by time(10s),container_name,hostname into net_stats_apps",
		"select container_name,derivative(rx_bytes + tx_bytes) as net_io from \"stats\" where container_name =~ %s group by time(10s),container_name,hostname into net_stats_infra",
		"select container_name,derivative(diskio_service_bytes) as disk_io from \"stats\" where container_name !~ %s group by time(10s),container_name,hostname into disk_stats_apps",
		"select container_name,derivative(diskio_service_bytes) as disk_io from \"stats\" where container_name =~ %s group by time(10s),container_name,hostname into disk_stats_infra",
	}
	queries := make(map[string]string)
	for _, q := range _queries {
		queries[seriesName(q)] = fmt.Sprintf(q, infra)
	}
	for {
		time.Sleep(10 * time.Second)
		// GetContinuousQueries -> 404
		series, err := client.Query("list continuous queries")
		if err != nil {
			glog.Errorf("Cannot obtain list of exists InfluxDB continuous queries: %v", err)
			continue
		}
		if len(series) > 0 {
			existing := make(map[string]int)
			for _, row := range series[0].GetPoints() {
				query := row[2].(string)
				existing[query] = int(row[1].(float64))
			}
			for existingQuery, id := range existing {
				table := seriesName(existingQuery)
				query, exist := queries[table]
				if !exist {
					if *argDropUnknown {
						dropContinuousQuery(client, id)
					}
				} else {
					if existingQuery == query {
						delete(queries, table)
					} else {
						dropContinuousQuery(client, id)
					}
				}
			}
		}
		if len(queries) == 0 {
			return
		}
		for _, query := range queries {
			_, err := client.Query(query)
			if err != nil {
				glog.Errorf("Cannot create InfluxDB continuous query `%s`: %v", query, err)
				continue
			}
		}
	}
}
