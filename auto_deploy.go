package main

import (
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

type AutoDeploy struct {
	ClientSet *kubernetes.Clientset
}

func (deploy* AutoDeploy) init() {
	config, err := clientcmd.BuildConfigFromFlags("", k3sConfigPath)
	if err != nil {
		logrus.Fatalln("集群config创建失败")
	}
	logrus.Info("集群config创建成功")

	deploy.ClientSet, err = kubernetes.NewForConfig(config)
	if err != nil {
		logrus.Fatalln("clientset创建失败")
	}
}

func (deploy* AutoDeploy) getAllPodsByImageName(imageName string) ([]v1.Pod, error)  {
	return nil, nil
}

func (deploy* AutoDeploy) StopAllPodsByImageName(imageName string) error {
	return nil
}