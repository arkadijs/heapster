package sources

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/golang/glog"
	cadvisorClient "github.com/google/cadvisor/client"
	cadvisor "github.com/google/cadvisor/info/v1"
)

type cadvisorSource struct {
	lastQuery    time.Time
	pollDuration time.Duration
}

func (self *cadvisorSource) processStat(hostname string, containerInfo *cadvisor.ContainerInfo) RawContainer {
	container := Container{
		Name:  containerInfo.Name,
		Spec:  containerInfo.Spec,
		Stats: containerInfo.Stats,
	}
	if len(containerInfo.Aliases) > 0 {
		container.Name = containerInfo.Aliases[0]
	}

	return RawContainer{hostname, container}
}

func (self *cadvisorSource) getAllCadvisorData(hostname, ip, port, container string) (containers []RawContainer, nodeInfo RawContainer, err error) {
	client, err := cadvisorClient.NewClient("http://" + ip + ":" + port)
	if err != nil {
		return
	}
	duration := time.Since(self.lastQuery)
	if duration < self.pollDuration {
		duration = self.pollDuration
	}
	allContainers, err := client.AllDockerContainers(
		&cadvisor.ContainerInfoRequest{NumStats: int(duration / time.Second)})
	if err != nil {
		glog.Errorf("Failed to get stats from cAdvisor on host `%s` with ip %s: %v\n", hostname, ip, err)
		return
	}

	for _, containerInfo := range allContainers {
		container := self.processStat(hostname, &containerInfo)
		if !strings.HasPrefix(container.Name, "k8s_") {
			containers = append(containers, container)
		}
	}

	return
}

func (self *cadvisorSource) fetchData(cadvisorHosts *CadvisorHosts) (rawContainers []RawContainer, nodesInfo []RawContainer, err error) {
	for hostname, ip := range cadvisorHosts.Hosts {
		containers, _, err := self.getAllCadvisorData(hostname, ip, strconv.Itoa(cadvisorHosts.Port), "/")
		if err != nil {
			return nil, nil, fmt.Errorf("Failed to get cAdvisor data from host `%s`: %v\n", hostname, err)
		}
		rawContainers = append(rawContainers, containers...)
	}
	self.lastQuery = time.Now()

	return
}

func newCadvisorSource() *cadvisorSource {
	return &cadvisorSource{
		lastQuery:    time.Now(),
		pollDuration: *ArgPollDuration,
	}
}
