package watch

import (
	"container/list"
	"runtime/debug"
	"strings"
	"time"

	"github.com/golang/glog"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/apimachinery/pkg/watch"
)

type NodeData struct {
	// core.NodeSystemInfo
	core.NodeStatus `json:",inline"`
	Name            string `json:"name"`
}

func (updateNode *NodeData) UpdateNodeData(node *core.Node) {
	updateNode.Name = node.ObjectMeta.Name
	updateNode.NodeStatus = node.Status
}

func UpdateNode(node *core.Node, ndm map[int]*list.List) *NodeData {

	var nd *NodeData
	for _, v := range ndm {
		if v == nil || v.Len() == 0 {
			continue
		}
		if strings.Compare(v.Front().Value.(*NodeData).Name, node.ObjectMeta.Name) == 0 {
			v.Front().Value.(*NodeData).UpdateNodeData(node)
			glog.Infof("node %s updated", v.Front().Value.(*NodeData).Name)
			nd = v.Front().Value.(*NodeData)
			break
		}
		if strings.Compare(v.Front().Value.(*NodeData).Name, node.ObjectMeta.GenerateName) == 0 {
			v.Front().Value.(*NodeData).UpdateNodeData(node)
			glog.Infof("node %s updated", v.Front().Value.(*NodeData).Name)
			nd = v.Front().Value.(*NodeData)
			break
		}
	}
	return nd
}

func RemoveNode(node *core.Node, ndm map[int]*list.List) string {

	var nodeName string
	for _, v := range ndm {
		if v == nil || v.Len() == 0 {
			continue
		}
		if strings.Compare(v.Front().Value.(*NodeData).Name, node.ObjectMeta.Name) == 0 {
			v.Remove(v.Front())
			glog.Infof("node %s updated", v.Front().Value.(*NodeData).Name)
			nodeName = v.Front().Value.(*NodeData).Name
			break
		}
		if strings.Compare(v.Front().Value.(*NodeData).Name, node.ObjectMeta.GenerateName) == 0 {
			v.Remove(v.Front())
			glog.Infof("node %s updated", v.Front().Value.(*NodeData).Name)
			nodeName = v.Front().Value.(*NodeData).Name
			break
		}
	}
	return nodeName
}

// NodeWatch Watching over nodes
func (wh *WatchHandler) NodeWatch() {
	defer func() {
		if err := recover(); err != nil {
			glog.Errorf("RECOVER NodeWatch. error: %v, stack: %s", err, debug.Stack())
		}
	}()
	var lastWatchEventCreationTime time.Time
	newStateChan := make(chan bool)
	wh.newStateReportChans = append(wh.newStateReportChans, newStateChan)
	for {
		wh.clusterAPIServerVersion = wh.getClusterVersion()
		wh.cloudVendor = wh.checkInstanceMetadataAPIVendor()
		if wh.cloudVendor != "" {
			wh.clusterAPIServerVersion.GitVersion += ";" + wh.cloudVendor
		}
		glog.Infof("K8s Cloud Vendor : %s", wh.cloudVendor)

		glog.Infof("Watching over nodes starting")
		nodesWatcher, err := wh.RestAPIClient.CoreV1().Nodes().Watch(globalHTTPContext, metav1.ListOptions{Watch: true})
		if err != nil {
			glog.Errorf("cannot watch over nodes. %v", err)
			time.Sleep(3 * time.Second)
			continue
		}
		wh.handleNodeWatch(nodesWatcher, newStateChan, &lastWatchEventCreationTime)

	}
}
func (wh *WatchHandler) handleNodeWatch(nodesWatcher watch.Interface, newStateChan <-chan bool, lastWatchEventCreationTime *time.Time) {
	nodesChan := nodesWatcher.ResultChan()
	for {
		var event watch.Event
		select {
		case event = <-nodesChan:
		case <-newStateChan:
			nodesWatcher.Stop()
			glog.Errorf("Node watch - newStateChan signal")
			*lastWatchEventCreationTime = time.Now()
			return
		}
		if event.Type == watch.Error {
			glog.Errorf("Node watch chan loop error: %v", event.Object)
			nodesWatcher.Stop()
			*lastWatchEventCreationTime = time.Now()
			return
		}
		if node, ok := event.Object.(*core.Node); ok {
			node.ManagedFields = []metav1.ManagedFieldsEntry{}
			switch event.Type {
			case "ADDED":
				if node.CreationTimestamp.Time.Before(*lastWatchEventCreationTime) {
					glog.Infof("node %s already exist, will not be reported", node.ObjectMeta.Name)
					continue
				}
				id := CreateID()
				if wh.ndm[id] == nil {
					wh.ndm[id] = list.New()
				}
				nd := &NodeData{Name: node.ObjectMeta.Name,
					NodeStatus: node.Status,
				}
				wh.ndm[id].PushBack(nd)
				informNewDataArrive(wh)
				wh.jsonReport.AddToJsonFormat(nd, NODE, CREATED)
			case "MODIFY":
				updateNode := UpdateNode(node, wh.ndm)
				informNewDataArrive(wh)
				wh.jsonReport.AddToJsonFormat(updateNode, NODE, UPDATED)
			case "DELETED":
				name := RemoveNode(node, wh.ndm)
				informNewDataArrive(wh)
				wh.jsonReport.AddToJsonFormat(name, NODE, DELETED)
			case "BOOKMARK": //only the resource version is changed but it's the same workload
				continue
			case "ERROR":
				glog.Errorf("while watching over nodes we got an error: %v", event)
				*lastWatchEventCreationTime = time.Now()
				return
			}
		} else {
			*lastWatchEventCreationTime = time.Now()
			return
		}
	}
}

func (wh *WatchHandler) checkInstanceMetadataAPIVendor() string {
	res, _ := getInstanceMetadata()
	return res
}

func (wh *WatchHandler) getClusterVersion() *version.Info {
	glog.Infof("Taking k8s API version")
	serverVersion, err := wh.RestAPIClient.Discovery().ServerVersion()
	if err != nil {
		serverVersion = &version.Info{GitVersion: "Unknown"}
	}
	glog.Infof("K8s API version: %v", serverVersion)
	return serverVersion
}
