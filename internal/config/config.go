package config

import (
	"time"

	"github.com/urfave/cli/v2"
)

type Config struct {
	// KubeConfigPath is the path to the kubeconfig file
	KubeConfigPath string `json:"kubeconfig"`
	// NodeName is the name of the Kubernetes node
	NodeName string `json:"node-name"`
	// Project is the name of the GCP project or the AWS account ID
	Project string `json:"project"`
	// Region is the name of the GCP region or the AWS region
	Region string `json:"region"`
	// DevelopMode mode
	DevelopMode bool `json:"develop-mode"`
	// Filter is the filter for the IP addresses
	Filter []string `json:"filter"`
	// OrderBy is the order by for the IP addresses
	OrderBy string `json:"order-by"`
	// Retry interval
	RetryInterval time.Duration `json:"retry-interval"`
	// Retry attempts
	RetryAttempts int `json:"retry-attempts"`
}

func NewConfig(c *cli.Context) *Config {
	var cfg Config
	cfg.KubeConfigPath = c.String("kubeconfig")
	cfg.NodeName = c.String("node-name")
	cfg.DevelopMode = c.Bool("develop-mode")
	cfg.RetryInterval = c.Duration("retry-interval")
	cfg.RetryAttempts = c.Int("retry-attempts")
	cfg.Filter = c.StringSlice("filter")
	cfg.OrderBy = c.String("order-by")
	cfg.Project = c.String("project")
	cfg.Region = c.String("region")
	return &cfg
}
