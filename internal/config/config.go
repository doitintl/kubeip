package config

import (
	"strings"
	"time"

	"github.com/spf13/viper"
)

const (
	defaultTicker = 5 * time.Minute
)

// Config kubeip configuration
type Config struct {
	KubeConfigPath      string
	LabelKey            string
	LabelValue          string
	NodePool            string
	ForceAssignment     bool
	AdditionalNodePools []string
	Ticker              time.Duration
	AllNodePools        bool
	OrderByLabelKey     string
	OrderByDesc         bool
	CopyLabels          bool
	ClearLabels         bool
	LogLevel            string
	LogJSON             bool
	DryRun              bool
}

func setConfigDefaults() {
	viper.SetDefault("KubeConfigPath", "")
	viper.SetDefault("LabelKey", "kubeip")
	viper.SetDefault("LabelValue", "reserved")
	viper.SetDefault("NodePool", "default-pool")
	viper.SetDefault("ForceAssignment", true)
	viper.SetDefault("AdditionalNodePools", "")
	viper.SetDefault("Ticker", defaultTicker)
	viper.SetDefault("AllNodePools", false)
	viper.SetDefault("OrderByLabelKey", "priority")
	viper.SetDefault("OrderByDesc", true)
	viper.SetDefault("CopyLabels", true)
	viper.SetDefault("ClearLabels", true)
	viper.SetDefault("LogLevel", "info")
	viper.SetDefault("LogJSON", false)
	viper.SetDefault("DryRun", false)
}

// NewConfig initialize kubeip configuration
func NewConfig() *Config {
	viper.SetEnvPrefix("kubeip")
	viper.AutomaticEnv()
	setConfigDefaults()

	var AdditionalNodePools []string
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
		OrderByLabelKey:     viper.GetString("orderbylabelkey"),
		OrderByDesc:         viper.GetBool("orderbydesc"),
		CopyLabels:          viper.GetBool("copylabels"),
		ClearLabels:         viper.GetBool("clearlabels"),
		DryRun:              viper.GetBool("dryrun"),
	}
	return &c
}
