// Copyright © 2018 Aviv Laufer <aviv.laufer@gmail.com>
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
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.



package main

import (
	"github.com/Sirupsen/logrus"
	c "github.com/doitintl/kip/pkg/client"
	"github.com/doitintl/kip/pkg/compute"
)

func main() {

	cluster ,err:=compute.ClusterName()
	if err !=nil {
		logrus.Info(err)
	}
	projectID, err := compute.ProjectName()
	if err !=nil {
		logrus.Info(err)
	}
	logrus.WithFields(logrus.Fields{
		"Cluster name": cluster,
		"Project name": projectID,
	}).Info(" starting")

	zones,_ :=compute.ListClusterZones("aviv-playground", "skid-master")
	for _, zone := range zones {
		// element is the element from someSlice for where we are
		logrus.Info(zone)
	}
	c.Run()

}
