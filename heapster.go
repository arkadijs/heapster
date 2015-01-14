package main

import (
	"flag"
	"os"
	"strings"
	"time"

	"github.com/GoogleCloudPlatform/heapster/sinks"
	"github.com/GoogleCloudPlatform/heapster/sources"
	"github.com/golang/glog"
)

const heapsterVersion = "0.5-dev"

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
	srcs, err := sources.NewSources()
	if err != nil {
		return err
	}
	sink, err := sinks.NewSink()
	if err != nil {
		return err
	}
	ticker := time.NewTicker(*sources.ArgPollDuration)
	defer ticker.Stop()
	dataChan := make(chan sources.Data, 10)
	for {
		select {
		case <-ticker.C:
			for _, source := range srcs {
				go func(source sources.Source) {
					data, err := source.GetInfo()
					if err != nil {
						glog.Errorf("Failed to retrieve stats: %v", err)
					} else {
						dataChan <- data
					}
				}(source)
			}

		case data := <-dataChan:
			if err := sink.StoreData(data); err != nil {
				glog.Error(err)
			}
		}
	}
}
