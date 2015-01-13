package sources

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"time"

	kube_api "github.com/GoogleCloudPlatform/kubernetes/pkg/api"
	kube_client "github.com/GoogleCloudPlatform/kubernetes/pkg/client"
	kube_labels "github.com/GoogleCloudPlatform/kubernetes/pkg/labels"
	"github.com/golang/glog"
	cadvisor "github.com/google/cadvisor/info"
)

type KubeSource struct {
	client       *kube_client.Client
	lastQuery    time.Time
	pollDuration time.Duration
	kubeletPort  string
}

type nodeList CadvisorHosts

// Returns a map of minion hostnames to their corresponding IPs.
func (self *KubeSource) listMinions() (*nodeList, error) {
	nodeList := &nodeList{
		Port:  *argCadvisorPort,
		Hosts: make(map[string]string, 0),
	}
	minions, err := self.client.Nodes().List()
	if err != nil {
		return nil, err
	}
	for _, minion := range minions.Items {
		addrs, err := net.LookupIP(minion.Name)
		if err == nil {
			nodeList.Hosts[minion.Name] = addrs[0].String()
		} else {
			glog.Errorf("Skipping host `%s` since looking up its IP failed: %v", minion.Name, err)
		}
	}

	return nodeList, nil
}

func (self *KubeSource) parsePod(pod *kube_api.Pod) *Pod {
	localPod := Pod{
		Name:       pod.Name,
		ID:         pod.UID,
		Hostname:   pod.Status.Host,
		Status:     string(pod.Status.Phase),
		PodIP:      pod.Status.PodIP,
		Labels:     make(map[string]string, 0),
		Containers: make([]*Container, 0),
	}
	for key, value := range pod.Labels {
		localPod.Labels[key] = value
	}
	for _, container := range pod.Spec.Containers {
		localContainer := newContainer()
		localContainer.Name = container.Name
		localPod.Containers = append(localPod.Containers, localContainer)
	}
	glog.V(2).Infof("found pod: %+v", localPod)

	return &localPod
}

// Returns a map of minion hostnames to the Pods running in them.
func (self *KubeSource) getPods() ([]Pod, error) {
	pods, err := self.client.Pods(kube_api.NamespaceAll).List(kube_labels.Everything())
	if err != nil {
		return nil, err
	}
	glog.V(1).Infof("got pods from api server %+v", pods)
	// TODO(vishh): Add API Version check. Fail if Kubernetes returns an invalid API Version.
	out := make([]Pod, 0)
	for _, pod := range pods.Items {
		pod := self.parsePod(&pod)
		out = append(out, *pod)
	}

	return out, nil
}

func (self *KubeSource) getStatsFromKubelet(hostIP string, podName string, podID string, containerName string, numStats int) (cadvisor.ContainerSpec, []*cadvisor.ContainerStats, error) {
	var containerInfo cadvisor.ContainerInfo
	body, err := json.Marshal(cadvisor.ContainerInfoRequest{NumStats: numStats})
	if err != nil {
		return cadvisor.ContainerSpec{}, []*cadvisor.ContainerStats{}, err
	}
	url := fmt.Sprintf("http://%s:%s%s", hostIP, self.kubeletPort, filepath.Join("/stats", podName, containerName))
	if containerName == "/" {
		url += "/"
	}
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return cadvisor.ContainerSpec{}, []*cadvisor.ContainerStats{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	err = PostRequestAndGetValue(http.DefaultClient, req, &containerInfo)
	if err != nil {
		glog.Errorf("failed to get stats from kubelet url: %s - %s\n", url, err)
		return cadvisor.ContainerSpec{}, []*cadvisor.ContainerStats{}, nil
	}

	return containerInfo.Spec, containerInfo.Stats, nil
}

func (self *KubeSource) getNodesInfo(numStats int) ([]RawContainer, error) {
	kubeNodes, err := self.listMinions()
	if err != nil {
		return []RawContainer{}, err
	}
	nodesInfo := []RawContainer{}
	for node, ip := range kubeNodes.Hosts {
		spec, stats, err := self.getStatsFromKubelet(ip, "", "", "/", numStats)
		if err != nil {
			return []RawContainer{}, err
		}
		if len(stats) > 0 {
			container := RawContainer{node, Container{"/", spec, stats}}
			nodesInfo = append(nodesInfo, container)
		}
	}

	return nodesInfo, nil
}

func (self *KubeSource) GetInfo() (ContainerData, error) {
	pods, err := self.getPods()
	if err != nil {
		return ContainerData{}, err
	}
	duration := time.Since(self.lastQuery)
	if duration < self.pollDuration {
		duration = self.pollDuration
	}
	numStats := int(duration / time.Second)
	for _, pod := range pods {
		addrs, err := net.LookupIP(pod.Hostname)
		if err != nil {
			glog.Errorf("Skipping host %s since looking up its IP failed - %s", pod.Hostname, err)
			continue
		}
		hostIP := addrs[0].String()
		for _, container := range pod.Containers {
			spec, stats, err := self.getStatsFromKubelet(hostIP, pod.Name, pod.ID, container.Name, numStats)
			if err != nil {
				return ContainerData{}, err
			}
			container.Stats = stats
			container.Spec = spec
		}
	}
	nodesInfo, err := self.getNodesInfo(numStats)
	if err != nil {
		return ContainerData{}, err
	}

	self.lastQuery = time.Now()

	return ContainerData{Pods: pods, Machine: nodesInfo}, nil
}

func newKubeSource() (*KubeSource, error) {
	if len(*argMaster) == 0 {
		return nil, fmt.Errorf("kubernetes_master flag not specified")
	}
	kubeClient := kube_client.NewOrDie(&kube_client.Config{
		Host:     "http://" + *argMaster,
		Version:  "v1beta1",
		Insecure: true,
	})

	return &KubeSource{
		client:       kubeClient,
		lastQuery:    time.Now(),
		pollDuration: *ArgPollDuration,
		kubeletPort:  *argKubeletPort,
	}, nil
}
