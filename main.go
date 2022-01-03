package main

import (
	"context"
	"encoding/json"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

var gAutoDeploy *AutoDeploy

func test(w http.ResponseWriter, req *http.Request)  {
	w.Write([]byte("test"))
}

func listAllImage(w http.ResponseWriter, req *http.Request)  {
	api := gAutoDeploy.ClientSet.CoreV1()

	pods, err := api.Pods("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		logrus.Panic(err.Error())
	}

	//	var container v1.Container = pods.Items[0].Spec.Containers[0]
	//	logrus.Info(container.Image)
	//

	imageNameList := make([]string, 0)
	for _, pod := range pods.Items {
		for _, container := range pod.Spec.Containers {
			logrus.Info(container.Image)
			imageNameList = append(imageNameList, container.Image)
		}
	}
	result, err := json.Marshal(imageNameList)

	if err != nil {
		w.Write([]byte("出错了"))
		return
	}

	w.Write(result)
}

func main() {
	deploy := new(AutoDeploy)
	gAutoDeploy = deploy

	deploy.init("nginx:1.14.2")

	//之前将所有的相关的dp处理掉，后面就没有partialName了
	dpList, err := deploy.StopAllPodsByImageName("nginx")

	time.Sleep(5*time.Second)
	if err == nil {
		deploy.WaitDeployListAvailable(dpList)
	}

	http.HandleFunc("/test", test)
	http.HandleFunc("/listAllImage", listAllImage)

	logrus.Panic(http.ListenAndServe("0.0.0.0:8080", nil))
}