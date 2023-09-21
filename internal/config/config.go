package config

import (
	"time"

	"github.com/urfave/cli/v2"
)

type Config struct {
	// KubeConfigPath is the path to the kubeconfig file
	KubeConfigPath string `json:"kubeconfig"`
	// ClusterName is the name of the EKS cluster
	ClusterName string `json:"cluster-name"`
	// DevelopMode mode
	DevelopMode bool `json:"develop-mode"`
	// Retry interval
	RetryInterval time.Duration `json:"retry-interval"`
	// Retry attempts
	RetryAttempts int `json:"retry-attempts"`
}

func LoadConfig(c *cli.Context) Config {
	var cfg Config
	cfg.KubeConfigPath = c.String("kubeconfig")
	cfg.ClusterName = c.String("cluster-name")
	cfg.DevelopMode = c.Bool("develop-mode")
	cfg.RetryInterval = c.Duration("retry-interval")
	return cfg
}
