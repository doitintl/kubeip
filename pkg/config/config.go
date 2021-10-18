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
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package config

import (
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config kubeip configuration
type Config struct {
	LabelKey            string
	LabelValue          string
	NodePool            string
	ForceAssignment     bool
	AdditionalNodePools []string
	Ticker              time.Duration
	AllNodePools        bool
}

func setConfigDefaults() {
	viper.SetDefault("LabelKey", "kubeip")
	viper.SetDefault("LabelValue", "reserved")
	viper.SetDefault("NodePool", "default-pool")
	viper.SetDefault("ForceAssignment", true)
	viper.SetDefault("ForceAssignment", true)
	viper.SetDefault("AdditionalNodePools", "")
	viper.SetDefault("Ticker", 5)
	viper.SetDefault("AllNodePools", false)
}

// NewConfig initialize kubeip configuration
func NewConfig() (*Config, error) {
	var AdditionalNodePools []string
	viper.SetEnvPrefix("kubeip")
	viper.AutomaticEnv()
	setConfigDefaults()
	AdditionalNodePoolsStr := viper.GetString("additionalnodepools")
	if len(AdditionalNodePoolsStr) > 0 {
		AdditionalNodePools = strings.Split(AdditionalNodePoolsStr, ",")
	}

	c := Config{
		LabelKey:            viper.GetString("labelkey"),
		LabelValue:          viper.GetString("labelvalue"),
		NodePool:            viper.GetString("nodepool"),
		ForceAssignment:     viper.GetBool("forceassignment"),
		AdditionalNodePools: AdditionalNodePools,
		Ticker:              viper.GetDuration("ticker"),
		AllNodePools:        viper.GetBool("allnodepools"),
	}
	return &c, nil
}
