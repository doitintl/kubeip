// Copyright © 2023 DoiT International
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
	"github.com/doitintl/kubeip/pkg/config"
	"github.com/doitintl/kubeip/pkg/controller"
	"github.com/doitintl/kubeip/pkg/kipcompute"
	"github.com/sirupsen/logrus"
)

var (
	version   string
	buildDate string
	gitCommit string
	gitBranch string
)

func main() {
	logger := logrus.New()
	cfg := config.NewConfig()
	logger.Info(cfg)

	cluster, err := kipcompute.ClusterName()
	if err != nil {
		logger.WithError(err).Fatal("Failed to get cluster name")
	}

	project, err := kipcompute.ProjectName()
	if err != nil {
		logger.WithError(err).Fatal("Failed to get project name")
	}

	logger.WithFields(logrus.Fields{
		"Cluster":    cluster,
		"Project":    project,
		"Version":    version,
		"Build Date": buildDate,
		"Git Commit": gitCommit,
		"Git Branch": gitBranch,
	}).Info("kubeIP is starting")

	if err = controller.Start(logger, project, cluster, cfg); err != nil {
		logrus.WithError(err).Fatal("Failed to start kubeIP controller")
	}
}
