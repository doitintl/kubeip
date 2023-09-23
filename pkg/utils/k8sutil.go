// Copyright © 2021 DoiT International
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package utils

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	cfg "github.com/doitintl/kubeip/pkg/config"
	"github.com/doitintl/kubeip/pkg/types"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	apiv1 "k8s.io/api/core/v1"
	extv1beta1 "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	typesv1 "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// Min helper method to determine the minimum between two numbers
func Min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Contains helper method to determine if a string is contained in an array
func Contains(s []string, e string) bool {
	for _, a := range s {
		if strings.EqualFold(a, e) {
			return true
		}
	}
	return false
}

// GetClient returns a k8s clientset to the request from inside of cluster
func GetClient() (kubernetes.Interface, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, errors.Wrap(err, "Can not get kubernetes config")
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, errors.Wrap(err, "Can not create kubernetes clientset")
	}

	return clientset, nil
}

// GetObjectMetaData returns metadata of a given k8s object
func GetObjectMetaData(obj interface{}) metav1.ObjectMeta {

	var objectMeta metav1.ObjectMeta

	switch object := obj.(type) {
	case *appsv1.Deployment:
		objectMeta = object.ObjectMeta
	case *apiv1.ReplicationController:
		objectMeta = object.ObjectMeta
	case *appsv1.ReplicaSet:
		objectMeta = object.ObjectMeta
	case *appsv1.DaemonSet:
		objectMeta = object.ObjectMeta
	case *apiv1.Service:
		objectMeta = object.ObjectMeta
	case *apiv1.Pod:
		objectMeta = object.ObjectMeta
	case *batchv1.Job:
		objectMeta = object.ObjectMeta
	case *apiv1.PersistentVolume:
		objectMeta = object.ObjectMeta
	case *apiv1.Namespace:
		objectMeta = object.ObjectMeta
	case *apiv1.Secret:
		objectMeta = object.ObjectMeta
	case *extv1beta1.Ingress:
		objectMeta = object.ObjectMeta
	}
	return objectMeta
}

func clearLabels(m map[string]string, config *cfg.Config) string {
	stringBuffer := new(bytes.Buffer)
	for key := range m {
		if !strings.EqualFold(key, config.OrderByLabelKey) &&
			!strings.EqualFold(key, config.LabelKey) &&
			!strings.Contains(key, "kubip_assigned") &&
			!strings.Contains(key, "kubernetes") &&
			!strings.Contains(key, "google") &&
			!strings.Contains(key, "gke") {
			fmt.Fprintf(stringBuffer, " ,\"%s\":null", key)
		}
	}
	return stringBuffer.String()
}

func createLabelKeyValuePairs(m map[string]string, config *cfg.Config) string {
	stringBuffer := new(bytes.Buffer)
	for key, value := range m {
		if !strings.EqualFold(key, config.OrderByLabelKey) &&
			!strings.EqualFold(key, config.LabelKey) &&
			!strings.Contains(key, "kubip_assigned") &&
			!strings.Contains(key, "kubernetes") &&
			!strings.Contains(key, "google") &&
			!strings.Contains(key, "gke") {
			fmt.Fprintf(stringBuffer, " ,\"%s\":\"%s\"", key, value)
		}
	}
	return stringBuffer.String()
}

// TagNode tag GKE node with "kubip_assigned" label (with typo) and also copy the labels present on the address if the copyLabels flag is set to true
func TagNode(node string, ip types.IPAddress, config *cfg.Config) error {
	kubeClient, err := GetClient()
	if err != nil {
		return errors.Wrap(err, "Can not get kubernetes API")
	}
	logrus.WithFields(logrus.Fields{"pkg": "kubeip", "function": "tagNode"}).Infof("Tagging node %s as %s", node, ip.IP)
	dashIP := strings.Replace(ip.IP, ".", "-", 4)
	var labelString string

	if config.CopyLabels {
		var labelsToClear string
		if config.ClearLabels {
			result, err := kubeClient.CoreV1().Nodes().Get(context.Background(), node, metav1.GetOptions{})
			if err != nil {
				logrus.WithFields(logrus.Fields{"pkg": "kubeip", "function": "tagNode"}).WithError(err).Warnf("Can not get node %s", node)
			} else {
				logrus.WithFields(logrus.Fields{"pkg": "kubeip", "function": "tagNode"}).Infof("Clear label tag for node %s with ip %s and clear tags %s", node, ip.IP, result.Labels)
				createLabelKeyValuePairs(result.Labels, config)
				labelsToClear = clearLabels(result.Labels, config)
			}
		} else {
			labelsToClear = ""
		}

		labelString = "{" + "\"" + "kubip_assigned" + "\":\"" + dashIP + "\"" + labelsToClear + createLabelKeyValuePairs(ip.Labels, config) + "}"
	} else {
		labelString = "{" + "\"" + "kubip_assigned" + "\":\"" + dashIP + "\"" + "}"
	}
	patch := fmt.Sprintf(`{"metadata":{"labels":%v}}`, labelString)

	if config.DryRun {
		logrus.WithFields(logrus.Fields{"pkg": "kubeip", "function": "tagNode"}).Infof("Tagging node %s as %s with tags %s ", node, ip.IP, labelString)
	} else {
		logrus.WithFields(logrus.Fields{"pkg": "kubeip", "function": "tagNode"}).Infof("Tagging node %s as %s with tags %s ", node, ip.IP, labelString)
		_, err = kubeClient.CoreV1().Nodes().Patch(context.Background(), node, typesv1.MergePatchType, []byte(patch), metav1.PatchOptions{})
		if err != nil {
			return errors.Wrap(err, "Can not patch node")
		}
	}
	return nil
}

// GetNodeByIP get GKE node by IP
func GetNodeByIP(ip string) (string, error) {
	kubeClient, err := GetClient()
	if err != nil {
		return "", errors.Wrap(err, "Can not get kubernetes API")
	}
	dashIP := strings.Replace(ip, ".", "-", 4)
	label := fmt.Sprintf("kubip_assigned=%v", dashIP)
	l, err := kubeClient.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{
		LabelSelector: label,
	})
	if err != nil {
		logrus.Error(err)
		return "", err
	}
	if len(l.Items) == 0 {
		return "", errors.New("did not found matching node with IP")
	}
	return l.Items[0].GetName(), nil

}

func isNodeReady(conditions []apiv1.NodeCondition) bool {
	for _, condition := range conditions {
		if condition.Type == apiv1.NodeReady {
			return condition.Status == apiv1.ConditionTrue
		}
	}
	return false
}

// WaitForNodeReady wait for node to be ready
func WaitForNodeReady(node string, timeout time.Duration) error {
	kubeClient, err := GetClient()
	if err != nil {
		return errors.Wrap(err, "Can not get kubernetes API")
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	for {
		select {
		case <-ctx.Done():
			return errors.New("timeout waiting for node to be ready")
		default:
			var options metav1.GetOptions
			options.Kind = "Node"
			options.APIVersion = "1"
			n, err := kubeClient.CoreV1().Nodes().Get(context.Background(), node, options)
			if err != nil {
				return errors.Wrap(err, "can not get node")
			}
			if isNodeReady(n.Status.Conditions) {
				return nil
			}
			time.Sleep(5 * time.Second)
		}
	}
}
