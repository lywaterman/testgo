package main

import (
	"context"
	"errors"
	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"strings"
	"sync"
	"time"
)

type AutoDeploy struct {
	ClientSet        *kubernetes.Clientset
	ImageName    string
}

func (deploy* AutoDeploy) init(ImangeName string) {
	config, err := clientcmd.BuildConfigFromFlags("", k3sConfigPath)
	if err != nil {
		logrus.Fatalln("集群config创建失败")
	}
	logrus.Info("集群config创建成功")

	deploy.ClientSet, err = kubernetes.NewForConfig(config)
	if err != nil {
		logrus.Fatalln("clientset创建失败")
	}

	deploy.ImageName = ImangeName
}

func (deploy* AutoDeploy) GetPodListByImageName(imageName string) ([]v1.Pod, error)  {
	api := deploy.ClientSet.CoreV1()

	pods, err := api.Pods("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	podList := make([]v1.Pod, 0)
	for _, pod := range pods.Items {
		for _, container := range pod.Spec.Containers {
			if strings.Contains(container.Image, imageName) {
				logrus.Info(container.Image)
				logrus.Info(pod.Name)
				podList = append(podList, pod)
			}
		}
	}
	return podList, nil
}

func (deploy *AutoDeploy) deployDebugSvc() (*v1.Service, error) {

	return nil, nil
}

func (deploy *AutoDeploy) getDeployment(dpName string, namespace string) (*appsv1.Deployment, error) {
	dp, err := deploy.ClientSet.AppsV1().Deployments(dpName).Get(context.TODO(), dpName, metav1.GetOptions{})

	if err != nil {
		return nil, err
	}

	return dp, nil
}

func (deploy *AutoDeploy) getDpIndicatorByPod(pod v1.Pod) (*DeployIndicator, error) {
	if len(pod.OwnerReferences) == 0 {
		logrus.Infof("pod %s has no owner", pod.Name)
		return nil, errors.New("noOwner")
	}

	switch pod.OwnerReferences[0].Kind{
	case "ReplicaSet"	:
		replica, err := deploy.ClientSet.AppsV1().ReplicaSets(pod.Namespace).Get(context.TODO(), pod.OwnerReferences[0].Name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}

		deployment, err := deploy.ClientSet.AppsV1().Deployments(pod.Namespace).Get(context.TODO(), replica.OwnerReferences[0].Name, metav1.GetOptions{})

		if err != nil {
			return nil, err
		}

		logrus.Infof("Pod's Deployment is %s", deployment.Name)
		return &DeployIndicator{immutable{Name: deployment.Name, Namespace: deployment.Namespace}}, nil
	}

	return nil, errors.New("ownerIsNotDP")
}

func (deploy *AutoDeploy) modifyDeployForDebug(dpi *DeployIndicator, imageName string) error {
	deployment, err := deploy.ClientSet.AppsV1().Deployments(dpi.Namespace).Get(context.TODO(), dpi.Name, metav1.GetOptions{})
	deploymentNew := *deployment

	deploymentNew.Spec.Template.Spec.RestartPolicy = v1.RestartPolicyAlways
	count := len(deploymentNew.Spec.Template.Spec.Containers)

	for i:=0; i<count; i++ {
		container := deploymentNew.Spec.Template.Spec.Containers[i]
		if strings.Contains(container.Image, imageName) {
			replica := int32(2)
			deploymentNew.Spec.Replicas = &replica
			deploymentNew.Spec.Template.Spec.Containers[i].ImagePullPolicy = v1.PullIfNotPresent
			deploymentNew.Spec.Template.Spec.Containers[i].Image = deploy.ImageName
			deploymentNew.Spec.Template.Labels["CustomDebug"] = "true"
		}
	}
	_, err = deploy.ClientSet.AppsV1().Deployments(dpi.Namespace).Update(context.TODO(), &deploymentNew, metav1.UpdateOptions{})
	if err != nil {
		return err
	}
	return nil
}

func (deploy *AutoDeploy) setScale(dpi *DeployIndicator, count int32) error {
	scale, err := deploy.ClientSet.AppsV1().Deployments(dpi.Namespace).GetScale(context.TODO(), dpi.Name, metav1.GetOptions{})

	if err != nil {
		return err
	}

	scaleNew := *scale
	scaleNew.Spec.Replicas = count

	_, err = deploy.ClientSet.AppsV1().Deployments(dpi.Namespace).UpdateScale(context.TODO(), dpi.Name, &scaleNew, metav1.UpdateOptions{})
	if err != nil {
		return err
	}
	return nil
}

func (deploy *AutoDeploy) WaitDeployListAvailable(dpiList []*DeployIndicator)  {
	var wg sync.WaitGroup
	for _, dpi := range dpiList {
		wg.Add(1)

		go func() {
			defer wg.Done()
			for true {
				deployment, _ := deploy.ClientSet.AppsV1().Deployments(dpi.Namespace).Get(context.TODO(), dpi.Name, metav1.GetOptions{})
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

	deploy.StartPortForward()
}

func (deploy *AutoDeploy) checkPodRunning(pod *v1.Pod) bool  {
	for _, status := range pod.Status.ContainerStatuses {
		if !status.Ready {
			return false
		}
	}

	return true
}

func (deploy *AutoDeploy) StartPortForward() error {
	api := deploy.ClientSet.CoreV1()

	var selectMap labels.Set = map[string]string{}
	selectMap["CustomDebug"] = "true"

	pods, err := api.Pods("").List(context.TODO(), metav1.ListOptions{LabelSelector: labels.SelectorFromSet(selectMap).String()})
	if err != nil {
		logrus.Info(err)
		return err
	}

	logrus.Infof("pods count is %d", len(pods.Items))

	desList := make(map[string][]int32)
	for _, pod := range pods.Items {
		if !(deploy.checkPodRunning(&pod)) {
			logrus.Infof("%s is not running", pod.Name)
			continue
		}
		for _, container := range pod.Spec.Containers {
			if container.Image == deploy.ImageName {
				logrus.Info(pod.Name)
				ports, prs := desList[pod.Name]
				if prs {
					ports = append(ports, container.Ports[0].ContainerPort)
				} else {
					portList := make([]int32, 0, 5)
					portList = append(portList, container.Ports[0].ContainerPort)
					desList[pod.Name] = portList
				}
			}
		}
	}

	logrus.Info(desList)

	for k, v := range desList {
		for _, port := range v {
			portPlus := rand.IntnRange(10000, 20000)
			ForwardPortToPod( int32(portPlus)+port, port, k)
		}
	}
	return nil
}


func (deploy *AutoDeploy) ShowDpListStatus(dpList []*appsv1.Deployment)  {
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
	podList, err := deploy.GetPodListByImageName(imageName)

	if err != nil {
		return nil, err
	}

	api := deploy.ClientSet.CoreV1()
	dpList := make([]*DeployIndicator, 0, 5)
	for _, pod := range podList{
		dpi, err := deploy.getDpIndicatorByPod(pod)

		if err != nil {
			continue
		}
		deploy.modifyDeployForDebug(dpi, imageName)
		err = api.Pods(pod.Namespace).Delete(context.TODO(), pod.Name, metav1.DeleteOptions{})

		logrus.Info(err)

		dpList = append(dpList, dpi)
	}
	return dpList, nil
}