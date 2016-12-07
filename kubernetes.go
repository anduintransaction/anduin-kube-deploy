package main

import (
	"gopkg.in/yaml.v2"
	"k8s.io/client-go/1.4/kubernetes"
	"k8s.io/client-go/1.4/pkg/api"
	"k8s.io/client-go/1.4/pkg/api/errors"
	"k8s.io/client-go/1.4/pkg/api/v1"
	"k8s.io/client-go/1.4/pkg/apis/apps/v1alpha1"
	v1batch "k8s.io/client-go/1.4/pkg/apis/batch/v1"
	"k8s.io/client-go/1.4/pkg/apis/extensions/v1beta1"
	"k8s.io/client-go/1.4/pkg/labels"
	"k8s.io/client-go/1.4/tools/clientcmd"
)

func loadKubernetesClient(config *appConfig) (*kubernetes.Clientset, error) {
	clientConfigLoader := &clientcmd.ClientConfigLoadingRules{
		ExplicitPath: config.configFile,
	}
	configOverrides := &clientcmd.ConfigOverrides{}
	if config.context != "" {
		configOverrides.CurrentContext = config.context
	}
	kubeConfig, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(clientConfigLoader, configOverrides).ClientConfig()
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(kubeConfig)
}

type KubernetesResource struct {
	Kind     string `yaml:"kind"`
	Metadata struct {
		Name      string `yaml:"name"`
		Namespace string `yaml:"namespace"`
	} `yaml:"metadata"`
}

func loadKubernetesResource(data []byte) (*KubernetesResource, error) {
	r := &KubernetesResource{}
	err := yaml.Unmarshal(data, r)
	if err != nil {
		return nil, err
	}
	return r, nil
}

func createNamespace(kubeClient *kubernetes.Clientset, namespace string) error {
	_, err := kubeClient.Core().Namespaces().Get(namespace)
	if err == nil {
		return nil
	}
	if !isResourceNotExist(err) {
		return err
	}
	ns := &v1.Namespace{
		ObjectMeta: v1.ObjectMeta{
			Name: namespace,
		},
	}
	_, err = kubeClient.Core().Namespaces().Create(ns)
	return err
}

func checkResourceExist(kubeClient *kubernetes.Clientset, kind, name, namespace string) (bool, error) {
	var err error
	switch kind {
	case "deployment":
		_, err = kubeClient.Extensions().Deployments(namespace).Get(name)
	case "service":
		_, err = kubeClient.Core().Services(namespace).Get(name)
	case "job":
		_, err = kubeClient.Batch().Jobs(namespace).Get(name)
	case "persistentvolumeclaim":
		_, err = kubeClient.Core().PersistentVolumeClaims(namespace).Get(name)
	case "configmap":
		_, err = kubeClient.Core().ConfigMaps(namespace).Get(name)
	case "petset":
		_, err = kubeClient.Apps().PetSets(namespace).Get(name)
	default:
		return false, UnsupportedResource(kind)
	}
	if err == nil {
		return true, nil
	}
	if isResourceNotExist(err) {
		return false, nil
	}
	return false, err
}

func createResource(kubeClient *kubernetes.Clientset, kind, namespace string, resourceData interface{}) error {
	var err error
	switch kind {
	case "deployment":
		_, err = kubeClient.Extensions().Deployments(namespace).Create(resourceData.(*v1beta1.Deployment))
	case "service":
		_, err = kubeClient.Core().Services(namespace).Create(resourceData.(*v1.Service))
	case "job":
		_, err = kubeClient.Batch().Jobs(namespace).Create(resourceData.(*v1batch.Job))
	case "persistentvolumeclaim":
		_, err = kubeClient.Core().PersistentVolumeClaims(namespace).Create(resourceData.(*v1.PersistentVolumeClaim))
	case "configmap":
		_, err = kubeClient.Core().ConfigMaps(namespace).Create(resourceData.(*v1.ConfigMap))
	case "petset":
		_, err = kubeClient.Apps().PetSets(namespace).Create(resourceData.(*v1alpha1.PetSet))
	default:
		return UnsupportedResource(kind)
	}
	return err
}

func destroyResource(kubeClient *kubernetes.Clientset, kind, name, namespace string) error {
	var err error
	deleteOptions := api.NewDeleteOptions(0)
	switch kind {
	case "deployment":
		return destroyDeployment(kubeClient, name, namespace)
	case "service":
		err = kubeClient.Core().Services(namespace).Delete(name, deleteOptions)
	case "job":
		return destroyJob(kubeClient, name, namespace)
	case "persistentvolumeclaim":
		err = kubeClient.Core().PersistentVolumeClaims(namespace).Delete(name, deleteOptions)
	case "configmap":
		err = kubeClient.Core().ConfigMaps(namespace).Delete(name, deleteOptions)
	case "petset":
		err = kubeClient.Apps().PetSets(namespace).Delete(name, deleteOptions)
	default:
		return UnsupportedResource(kind)
	}
	return err
}

func destroyDeployment(kubeClient *kubernetes.Clientset, name, namespace string) error {
	deleteOptions := api.NewDeleteOptions(0)
	err := kubeClient.Extensions().Deployments(namespace).Delete(name, deleteOptions)
	if err != nil {
		return err
	}
	selector, err := labels.Parse("name=" + name)
	if err != nil {
		return err
	}
	replicaSets, err := kubeClient.Extensions().ReplicaSets(namespace).List(api.ListOptions{
		LabelSelector: selector,
	})
	if err != nil {
		return err
	}
	for _, replicaSet := range replicaSets.Items {
		err = kubeClient.Extensions().ReplicaSets(namespace).Delete(replicaSet.Name, deleteOptions)
		if err != nil {
			return err
		}
	}
	return nil
}

func destroyJob(kubeClient *kubernetes.Clientset, name, namespace string) error {
	deleteOptions := api.NewDeleteOptions(0)
	err := kubeClient.Batch().Jobs(namespace).Delete(name, deleteOptions)
	if err != nil {
		return err
	}
	selector, err := labels.Parse("job-name=" + name)
	if err != nil {
		return err
	}
	pods, err := kubeClient.Core().Pods(namespace).List(api.ListOptions{
		LabelSelector: selector,
	})
	if err != nil {
		return err
	}
	for _, pod := range pods.Items {
		err = kubeClient.Core().Pods(namespace).Delete(pod.Name, deleteOptions)
		if err != nil {
			return err
		}
	}
	return nil
}

func isResourceNotExist(err error) bool {
	switch err := err.(type) {
	case *errors.StatusError:
		if err.Status().Code == 404 {
			return true
		}
		return false
	default:
		return false
	}
}