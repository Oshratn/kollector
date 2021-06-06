package watch

import (
	"container/list"
	"log"
	"strings"
	"time"

	"github.com/golang/glog"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
)

type ServiceData struct {
	Service *core.Service `json:",inline"`
}

func UpdateService(service *core.Service, sdm map[int]*list.List) string {
	for _, v := range sdm {
		if v == nil || v.Len() == 0 {
			continue
		}
		if strings.Compare(v.Front().Value.(ServiceData).Service.ObjectMeta.Name, service.ObjectMeta.Name) == 0 {
			*v.Front().Value.(ServiceData).Service = *service
			log.Printf("service %s updated", v.Front().Value.(ServiceData).Service.ObjectMeta.Name)
			return v.Front().Value.(ServiceData).Service.ObjectMeta.Name
		}
		if strings.Compare(v.Front().Value.(ServiceData).Service.ObjectMeta.GenerateName, service.ObjectMeta.Name) == 0 {
			*v.Front().Value.(ServiceData).Service = *service
			log.Printf("service %s updated", v.Front().Value.(ServiceData).Service.ObjectMeta.Name)
			return v.Front().Value.(ServiceData).Service.ObjectMeta.Name
		}
	}
	return ""
}

// RemoveService update websocket when service is removed
func RemoveService(service *core.Service, sdm map[int]*list.List) string {
	for _, v := range sdm {
		if v == nil || v.Len() == 0 {
			continue
		}
		if strings.Compare(v.Front().Value.(ServiceData).Service.ObjectMeta.Name, service.ObjectMeta.Name) == 0 {
			name := v.Front().Value.(ServiceData).Service.ObjectMeta.Name
			v.Remove(v.Front())
			log.Printf("service %s removed", name)
			return name
		}
		if strings.Compare(v.Front().Value.(ServiceData).Service.ObjectMeta.GenerateName, service.ObjectMeta.Name) == 0 {
			gName := v.Front().Value.(ServiceData).Service.ObjectMeta.Name
			v.Remove(v.Front())
			log.Printf("service %s removed", gName)
			return gName
		}
	}
	return ""
}

// ServiceWatch watch over servises
func (wh *WatchHandler) ServiceWatch(namespace string) {
	defer func() {
		if err := recover(); err != nil {
			log.Printf("RECOVER ServiceWatch. error: %v", err)
		}
	}()
	newStateChan := make(chan bool)
	wh.newStateReportChans = append(wh.newStateReportChans, newStateChan)
WatchLoop:
	for {
		log.Printf("Watching over services starting")
		serviceWatcher, err := wh.RestAPIClient.CoreV1().Services(namespace).Watch(globalHTTPContext, metav1.ListOptions{Watch: true})
		if err != nil {
			log.Printf("Cannot wathching over services. %v", err)
			time.Sleep(time.Duration(3) * time.Second)
			continue
		}
		serviceChan := serviceWatcher.ResultChan()
		log.Printf("Watching over services started")
	ChanLoop:
		for {
			var event watch.Event
			select {
			case event = <-serviceChan:
			case <-newStateChan:
				serviceWatcher.Stop()
				glog.Errorf("Service watch - newStateChan signal")
				continue WatchLoop
			}
			if event.Type == watch.Error {
				glog.Errorf("Service watch chan loop error: %v", event.Object)
				serviceWatcher.Stop()
				break ChanLoop
			}
			if service, ok := event.Object.(*core.Service); ok {
				service.ManagedFields = []metav1.ManagedFieldsEntry{}
				switch event.Type {
				case "ADDED":
					id := CreateID()
					if wh.sdm[id] == nil {
						wh.sdm[id] = list.New()
					}
					sd := ServiceData{Service: service}
					wh.sdm[id].PushBack(sd)
					informNewDataArrive(wh)
					wh.jsonReport.AddToJsonFormat(service, SERVICES, CREATED)
				case "MODIFY":
					UpdateService(service, wh.sdm)
					informNewDataArrive(wh)
					wh.jsonReport.AddToJsonFormat(service, SERVICES, UPDATED)
				case "DELETED":
					RemoveService(service, wh.sdm)
					informNewDataArrive(wh)
					wh.jsonReport.AddToJsonFormat(service, SERVICES, DELETED)
				case "BOOKMARK": //only the resource version is changed but it's the same workload
					continue
				case "ERROR":
					log.Printf("while watching over services we got an error: ")
				}
			} else {
				log.Printf("Got unexpected service from chan: %t, %v", event.Object, event.Object)
				break
			}
		}
		log.Printf("Wathching over services ended - since we got timeout")
	}
}
