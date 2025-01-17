package main

import (
	"flag"
	"log"
	"os"

	"github.com/kubescape/kollector/watch"

	"github.com/armosec/utils-k8s-go/probes"
	"github.com/golang/glog"
)

func main() {

	isServerReady := false
	go probes.InitReadinessV1(&isServerReady)
	displayBuildTag()

	wh, err := watch.CreateWatchHandler()
	if err != nil {
		log.Fatalf("failed to initialize the WatchHandler, reason: %s", err.Error())
	}

	go func() {
		for {
			wh.ListenerAndSender()
		}
	}()

	go func() {
		for {
			wh.PodWatch()
		}
	}()

	go func() {
		for {
			wh.NodeWatch()
		}
	}()

	go func() {
		for {
			wh.ServiceWatch()
		}
	}()

	go func() {
		for {
			wh.SecretWatch()
		}
	}()
	go func() {
		for {
			wh.NamespaceWatch()
		}
	}()
	go func() {
		for {
			wh.CronJobWatch()
		}
	}()
	glog.Error(wh.WebSocketHandle.SendReportRoutine(&isServerReady, wh.SetFirstReportFlag))

}

func displayBuildTag() {
	flag.Parse()
	glog.Infof("Image version: %s", os.Getenv("RELEASE"))
}
