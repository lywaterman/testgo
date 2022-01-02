package main

import (
	"context"
	"errors"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"strings"
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

func (deploy* AutoDeploy) GetPodNamesByImageName(imageName string, ignoreVersion bool) ([]v1.Pod, error)  {
	api := gAutoDeploy.ClientSet.CoreV1()

	pods, err := api.Pods("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	podList := make([]v1.Pod, 0)
	for _, pod := range pods.Items {
		for _, container := range pod.Spec.Containers {
			containerImageName := ""
			if ignoreVersion {
				imageNameArr := strings.Split(container.Image, ":")
				containerImageName = imageNameArr[0]
			} else {
				containerImageName = container.Image
			}
			if containerImageName == imageName {
				logrus.Info(container.Image)
				logrus.Info(pod.GenerateName)
				podList = append(podList, pod)
			}
		}
	}
	return podList, nil
}

func (deploy AutoDeploy) watchPod(namespace string) error {
	watch, _ := gAutoDeploy.ClientSet.CoreV1().Pods(namespace).Watch(context.TODO(), metav1.ListOptions{})
	go func() {
		for ;; {
			event :=  <- watch.ResultChan()
			logrus.Info(event.Type)
		}
	}()

	return nil
}

func (deploy AutoDeploy) getDeploymentName(pod v1.Pod) (string, error) {
	if len(pod.OwnerReferences) == 0 {
		logrus.Infof("pod %s has no owner", pod.Name)
		return "", errors.New("noOwner")
	}

	switch pod.OwnerReferences[0].Kind{
	case "ReplicaSet"	:
		replica, err := gAutoDeploy.ClientSet.AppsV1().ReplicaSets(pod.Namespace).Get(context.TODO(), pod.OwnerReferences[0].Name, metav1.GetOptions{})
		if err != nil {
			return "", err
		}

		deployment, err := gAutoDeploy.ClientSet.AppsV1().Deployments(pod.Namespace).Get(context.TODO(), replica.OwnerReferences[0].Name, metav1.GetOptions{})

		if err != nil {
			return "", err
		}

		logrus.Infof("Pod's Deployment is %s", deployment.Name)

		return deployment.Name, nil
	}

	return "", nil
}

func (deploy AutoDeploy) setAlwaysRestartAndPullImage(pod v1.Pod) error {
	deploymentName, err := deploy.getDeploymentName(pod)

	if err != nil {
		return nil
	}

	deployment, err := gAutoDeploy.ClientSet.AppsV1().Deployments(pod.Namespace).Get(context.TODO(), deploymentName, metav1.GetOptions{})

	if err != nil {
		return err
	}

	deploymentNew := *deployment

	deploymentNew.Spec.Template.Spec.RestartPolicy = v1.RestartPolicyAlways
	count := len(deploymentNew.Spec.Template.Spec.Containers)

	for i:=0; i<count; i++ {
		deploymentNew.Spec.Template.Spec.Containers[i].ImagePullPolicy = v1.PullAlways
	}
	_, err = gAutoDeploy.ClientSet.AppsV1().Deployments(pod.Namespace).Update(context.TODO(), &deploymentNew, metav1.UpdateOptions{})
	if err != nil {
		return err
	}
	return nil
}

func (deploy AutoDeploy) setScale(pod v1.Pod, count int32) error {
	deploymentName, err := deploy.getDeploymentName(pod)

	if err != nil {
		return nil
	}

	scale, err := gAutoDeploy.ClientSet.AppsV1().Deployments(pod.Namespace).GetScale(context.TODO(), deploymentName, metav1.GetOptions{})

	if err != nil {
		return err
	}

	scaleNew := *scale
	scaleNew.Spec.Replicas = count

	_, err = gAutoDeploy.ClientSet.AppsV1().Deployments(pod.Namespace).UpdateScale(context.TODO(), deploymentName, &scaleNew, metav1.UpdateOptions{})
	if err != nil {
		return err
	}
	return nil
}

func (deploy* AutoDeploy) StopAllPodsByImageName(imageName string) error {
	podList, err := deploy.GetPodNamesByImageName(imageName, true)

	if err != nil {
		return err
	}

	api := gAutoDeploy.ClientSet.CoreV1()

	for _, pod := range podList{
		err = deploy.setScale(pod, 1)
		err = deploy.setAlwaysRestartAndPullImage(pod)
		logrus.Info(err)
		err := api.Pods(pod.Namespace).Delete(context.TODO(), pod.Name, metav1.DeleteOptions{})
		if err != nil {
			logrus.Info(err)
			return err
		}
	}
	return nil
}