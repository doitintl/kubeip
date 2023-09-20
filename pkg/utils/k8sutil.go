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

	"github.com/doitintl/kubeip/pkg/config"
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
func GetClient() kubernetes.Interface {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		logrus.Fatalf("Can not get kubernetes config: %v", err)
	}

	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		logrus.Fatalf("Can not create kubernetes client: %v", err)
	}

	return clientset
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

func clearLabels(m map[string]string, cfg *config.Config) string {
	stringBuffer := new(bytes.Buffer)
	for key := range m {
		if !strings.EqualFold(key, cfg.OrderByLabelKey) &&
			!strings.EqualFold(key, cfg.LabelKey) &&
			!strings.Contains(key, "kubip_assigned") &&
			!strings.Contains(key, "kubernetes") &&
			!strings.Contains(key, "google") &&
			!strings.Contains(key, "gke") {
			fmt.Fprintf(stringBuffer, " ,\"%s\":null", key)
		}
	}
	return stringBuffer.String()
}

func createLabelKeyValuePairs(m map[string]string, cfg *config.Config) string {
	stringBuffer := new(bytes.Buffer)
	for key, value := range m {
		if !strings.EqualFold(key, cfg.OrderByLabelKey) &&
			!strings.EqualFold(key, cfg.LabelKey) &&
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
func TagNode(node string, ip *types.IPAddress, cfg *config.Config) {
	kubeClient := GetClient()
	logrus.WithFields(logrus.Fields{"pkg": "kubeip", "function": "tagNode"}).Infof("Tagging node %s as %s", node, ip.IP)
	// replace . with - in IP address
	dashIP := strings.Replace(ip.IP, ".", "-", 4) //nolint:gomnd
	var labelString string

	if cfg.CopyLabels {
		var labelsToClear string
		if cfg.ClearLabels {
			result, err := kubeClient.CoreV1().Nodes().Get(context.Background(), node, metav1.GetOptions{})
			if err != nil {
				logrus.WithFields(logrus.Fields{"pkg": "kubeip", "function": "tagNode"}).Error(err)
			} else {
				logrus.WithFields(logrus.Fields{"pkg": "kubeip", "function": "tagNode"}).Infof("Clear label tag for node %s with ip %s and clear tags %s", node, ip.IP, result.Labels)
				createLabelKeyValuePairs(result.Labels, cfg)
				labelsToClear = clearLabels(result.Labels, cfg)
			}
		} else {
			labelsToClear = ""
		}

		labelString = "{" + "\"" + "kubip_assigned" + "\":\"" + dashIP + "\"" + labelsToClear + createLabelKeyValuePairs(ip.Labels, cfg) + "}"
	} else {
		labelString = "{" + "\"" + "kubip_assigned" + "\":\"" + dashIP + "\"" + "}"
	}
	patch := fmt.Sprintf(`{"metadata":{"labels":%v}}`, labelString)

	if cfg.DryRun {
		logrus.WithFields(logrus.Fields{"pkg": "kubeip", "function": "tagNode"}).Infof("Tagging node %s as %s with tags %s ", node, ip.IP, labelString)
	} else {
		logrus.WithFields(logrus.Fields{"pkg": "kubeip", "function": "tagNode"}).Infof("Tagging node %s as %s with tags %s ", node, ip.IP, labelString)
		_, err := kubeClient.CoreV1().Nodes().Patch(context.Background(), node, typesv1.MergePatchType, []byte(patch), metav1.PatchOptions{})
		if err != nil {
			logrus.WithFields(logrus.Fields{"pkg": "kubeip", "function": "tagNode"}).Infof("Error occurred while tagging node %s as %s with tags %s ", node, ip.IP, labelString)
			logrus.Error(err)
		}
	}
}

// GetNodeByIP get GKE node by IP
func GetNodeByIP(ip string) (string, error) {
	kubeClient := GetClient()
	dashIP := strings.Replace(ip, ".", "-", 4) //nolint:gomnd
	label := fmt.Sprintf("kubip_assigned=%v", dashIP)
	l, err := kubeClient.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{
		LabelSelector: label,
	})
	if err != nil {
		return "", errors.Wrap(err, "failed to list nodes")
	}
	if len(l.Items) == 0 {
		return "", errors.New("did not found matching node with IP")
	}
	return l.Items[0].GetName(), nil
}
