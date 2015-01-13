package sources

import (
	"net"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/golang/glog"

	fleet "github.com/coreos/fleet/client"
	"github.com/coreos/fleet/etcd"
	fleetPkg "github.com/coreos/fleet/pkg"
	"github.com/coreos/fleet/registry"
)

type ExternalSource struct {
	cadvisor *cadvisorSource
	hosts    atomic.Value
}

func (self *ExternalSource) GetInfo() (ContainerData, error) {
	containers, nodes, err := self.cadvisor.fetchData(self.hosts.Load().(*CadvisorHosts))
	if err != nil {
		glog.Error(err)
		return ContainerData{}, nil
	}

	return ContainerData{
		Containers: containers,
		Machine:    nodes,
	}, nil
}

func newExternalSource(kubeSource *KubeSource) (Source, error) {
	cadvisorSource := newCadvisorSource()
	source := ExternalSource{
		cadvisor: cadvisorSource,
	}
	source.hosts.Store(&CadvisorHosts{})
	go updateHosts(&source.hosts, kubeSource)
	return &source, nil
}

func updateHosts(hosts *atomic.Value, kubeSource *KubeSource) {
	var _fleet fleet.API
	if kubeSource == nil {
		f, err := getFleetRegistryClient()
		if err != nil {
			glog.Errorf("Cannot create Fleet / ETCD client: %v", err)
			os.Exit(1)
		}
		_fleet = f
	}
	delay := 60
	for {
		if kubeSource != nil {
			nodes, err := kubeSource.listMinions()
			if err != nil {
				glog.Errorf("Error fetching list of cluster hosts from Kubernetes: %v", err)
				delay = 5
			} else {
				_hosts := CadvisorHosts(*nodes)
				hosts.Store(&_hosts)
				delay = 60
			}
		} else {
			machines, err := _fleet.Machines()
			if err != nil {
				glog.Errorf("Error fetching list of cluster hosts from Fleet / ETCD: %v", err)
				delay = 5
			} else {
				_machines := make(map[string]string)
				for _, machine := range machines {
					_machines[machine.ID] = machine.PublicIP
				}
				_hosts := CadvisorHosts{
					Port:  *argCadvisorPort,
					Hosts: _machines,
				}
				hosts.Store(&_hosts)
				delay = 60
			}
		}
		time.Sleep(time.Duration(delay) * time.Second)
	}
}

func getFleetRegistryClient() (fleet.API, error) {
	var dial func(string, string) (net.Conn, error)

	tlsConfig, err := fleetPkg.ReadTLSConfigFiles("", "", "")
	if err != nil {
		return nil, err
	}
	trans := &http.Transport{
		Dial:            dial,
		TLSClientConfig: tlsConfig,
	}
	eClient, err := etcd.NewClient(strings.Split(*argEtcd, ","), trans, 3*time.Second)
	if err != nil {
		return nil, err
	}
	reg := registry.NewEtcdRegistry(eClient, "/_coreos.com/fleet/")

	return &fleet.RegistryClient{Registry: reg}, nil
}
