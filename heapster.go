package main

import (
	"flag"
	"os"
	"strings"
	"time"

	"github.com/GoogleCloudPlatform/heapster/sinks"
	srcs "github.com/GoogleCloudPlatform/heapster/sources"
	"github.com/golang/glog"
)

func main() {
	flag.Parse()
	glog.Infof(strings.Join(os.Args, " "))
	glog.Infof("Heapster version %v https://github.com/arkadijs/heapster", heapsterVersion)
	err := doWork()
	if err != nil {
		glog.Error(err)
		os.Exit(1)
	}
	os.Exit(0)
}

func doWork() error {
	sources, err := srcs.NewSources()
	if err != nil {
		return err
	}
	sink, err := sinks.NewSink()
	if err != nil {
		return err
	}
	ticker := time.NewTicker(*srcs.ArgPollDuration)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			for _, source := range sources {
				data, err := source.GetInfo()
				if err != nil {
					glog.Error(err)
					continue
				}
				if err := sink.StoreData(data); err != nil {
					glog.Error(err)
				}
			}
		}
	}
}
