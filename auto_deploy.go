package main

import (
	"context"
	"errors"
	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"strings"
	"sync"
	"time"
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

func (deploy* AutoDeploy) GetPodListByImageName(imageName string, ignoreVersion bool) ([]v1.Pod, error)  {
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

func (deploy AutoDeploy) deployDebugSvc() (*v1.Service, error) {

	return nil, nil
}

func (deploy AutoDeploy) getDeployment(dpName string, namespace string) (*appsv1.Deployment, error) {
	dp, err := gAutoDeploy.ClientSet.AppsV1().Deployments(dpName).Get(context.TODO(), dpName, metav1.GetOptions{})

	if err != nil {
		return nil, err
	}

	return dp, nil
}

func (deploy AutoDeploy) getDpIndicatorByPod(pod v1.Pod) (*DeployIndicator, error) {
	if len(pod.OwnerReferences) == 0 {
		logrus.Infof("pod %s has no owner", pod.Name)
		return nil, errors.New("noOwner")
	}

	switch pod.OwnerReferences[0].Kind{
	case "ReplicaSet"	:
		replica, err := gAutoDeploy.ClientSet.AppsV1().ReplicaSets(pod.Namespace).Get(context.TODO(), pod.OwnerReferences[0].Name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}

		deployment, err := gAutoDeploy.ClientSet.AppsV1().Deployments(pod.Namespace).Get(context.TODO(), replica.OwnerReferences[0].Name, metav1.GetOptions{})

		if err != nil {
			return nil, err
		}

		logrus.Infof("Pod's Deployment is %s", deployment.Name)
		return &DeployIndicator{immutable{Name: deployment.Name, Namespace: deployment.Namespace}}, nil
	}

	return nil, errors.New("ownerIsNotDP")
}

func (deploy AutoDeploy) setAlwaysRestartAndPullImage(dpi *DeployIndicator) error {
	deployment, err := gAutoDeploy.ClientSet.AppsV1().Deployments(dpi.Namespace).Get(context.TODO(), dpi.Name, metav1.GetOptions{})
	deploymentNew := *deployment

	deploymentNew.Spec.Template.Spec.RestartPolicy = v1.RestartPolicyAlways
	count := len(deploymentNew.Spec.Template.Spec.Containers)

	for i:=0; i<count; i++ {
		deploymentNew.Spec.Template.Spec.Containers[i].ImagePullPolicy = v1.PullAlways
	}
	_, err = gAutoDeploy.ClientSet.AppsV1().Deployments(dpi.Namespace).Update(context.TODO(), &deploymentNew, metav1.UpdateOptions{})
	if err != nil {
		return err
	}
	return nil
}

func (deploy AutoDeploy) setScale(dpi *DeployIndicator, count int32) error {
	scale, err := gAutoDeploy.ClientSet.AppsV1().Deployments(dpi.Namespace).GetScale(context.TODO(), dpi.Name, metav1.GetOptions{})

	if err != nil {
		return err
	}

	scaleNew := *scale
	scaleNew.Spec.Replicas = count

	_, err = gAutoDeploy.ClientSet.AppsV1().Deployments(dpi.Namespace).UpdateScale(context.TODO(), dpi.Name, &scaleNew, metav1.UpdateOptions{})
	if err != nil {
		return err
	}
	return nil
}

func (deploy AutoDeploy) WaitDeployListAvailable(dpiList []*DeployIndicator)  {
	var wg sync.WaitGroup
	for _, dpi := range dpiList {
		wg.Add(1)

		go func() {
			defer wg.Done()
			for true {
				deployment, _ := gAutoDeploy.ClientSet.AppsV1().Deployments(dpi.Namespace).Get(context.TODO(), dpi.Name, metav1.GetOptions{})
				if deployment.Status.UnavailableReplicas == 0 {
					return
				} else {
					logrus.Infof("Deployment %s is not ready, will check after %d s", dpi.Name, 1)
					time.Sleep(time.Second)
				}
			}
		}()
	}

	wg.Wait()
	logrus.Info("启动完成")
}

func (deploy AutoDeploy) ShowDpListStatus(dpList []*appsv1.Deployment)  {
	for _, dp := range dpList {
		go func() {
			for true {
				logrus.Info(dp.Status)
				time.Sleep(time.Second)
			}
		}()
	}
}

func (deploy* AutoDeploy) StopAllPodsByImageName(imageName string) ([]*DeployIndicator, error) {
	podList, err := deploy.GetPodListByImageName(imageName, true)

	if err != nil {
		return nil, err
	}

	api := gAutoDeploy.ClientSet.CoreV1()
	dpList := make([]*DeployIndicator, 0, 5)
	for _, pod := range podList{
		dpi, err := deploy.getDpIndicatorByPod(pod)

		if err != nil {
			continue
		}
		deploy.setScale(dpi, 1)
		deploy.setAlwaysRestartAndPullImage(dpi)
		err = api.Pods(pod.Namespace).Delete(context.TODO(), pod.Name, metav1.DeleteOptions{})

		logrus.Info(err)

		dpList = append(dpList, dpi)
	}
	return dpList, nil
}