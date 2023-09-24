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

package main

import (
	c "github.com/doitintl/kubeip/pkg/client"
	cfg "github.com/doitintl/kubeip/pkg/config"
	"github.com/doitintl/kubeip/pkg/kipcompute"
	"github.com/sirupsen/logrus"
)

var config *cfg.Config
var version string
var buildDate string

func main() {
	config, _ = cfg.NewConfig()
	logrus.Info("kubeIP version: ", version)
	logrus.Info(config)
	cluster, err := kipcompute.ClusterName()
	if err != nil {
		logrus.Fatal(err)
		panic(err)
	}
	projectID, err := kipcompute.ProjectName()
	if err != nil {
		logrus.Fatal(err)
		panic(err)
	}
	logrus.Info(config.AdditionalNodePools)
	logrus.WithFields(logrus.Fields{
		"Cluster name": cluster,
		"Project name": projectID,
		"Version":      version,
		"Build Date":   buildDate,
	}).Info("kubeIP is starting")
	err = c.Run(config)
	if err != nil {
		logrus.Fatal(err)
		panic(err)
	}
}
