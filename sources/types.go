package sources

import (
	"flag"
	cadvisor "github.com/google/cadvisor/info"
	"time"
)

var (
	ArgPollDuration = flag.Duration("poll_duration", 10*time.Second, "Polling duration")
	argMaster       = flag.String("kubernetes_master", "", "Kubernetes master IP")
	argKubeletPort  = flag.String("kubelet_port", "10250", "Kubelet port")
)

type Data interface{}

type Container struct {
	Name  string                     `json:"name,omitempty"`
	Spec  cadvisor.ContainerSpec     `json:"spec,omitempty"`
	Stats []*cadvisor.ContainerStats `json:"stats,omitempty"`
}

func newContainer() *Container {
	return &Container{Stats: make([]*cadvisor.ContainerStats, 0)}
}

// PodState is the state of a pod, used as either input (desired state) or output (current state)
type Pod struct {
	Name       string            `json:"name,omitempty"`
	ID         string            `json:"id,omitempty"`
	Hostname   string            `json:"hostname,omitempty"`
	Containers []*Container      `json:"containers"`
	Status     string            `json:"status,omitempty"`
	PodIP      string            `json:"podIP,omitempty"`
	Labels     map[string]string `json:"labels,omitempty"`
}

type RawContainer struct {
	Hostname string `json:"hostname,omitempty"`
	Container
}

type ContainerData struct {
	Pods       []Pod
	Containers []RawContainer
	Machine    []RawContainer
}

type CadvisorHosts struct {
	Port  int               `json:"port"`
	Hosts map[string]string `json:"hosts"`
}

type Source interface {
	GetInfo() (ContainerData, error)
}

func NewSources() ([]Source, error) {
	if len(*argMaster) > 0 {
		kube, err := newKubeSource()
		if err != nil {
			return nil, err
		}
		ext, err := newExternalSource(kube)
		if err != nil {
			return nil, err
		}

		return []Source{kube, ext}, nil
	} else {
		ext, err := newExternalSource(nil)
		if err != nil {
			return nil, err
		}
		return []Source{ext}, nil
	}
}
