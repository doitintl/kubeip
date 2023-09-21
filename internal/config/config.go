package config

import (
	"github.com/urfave/cli/v2"
)

type Config struct {
	// KubeConfigPath is the path to the kubeconfig file
	KubeConfigPath string `json:"kubeconfig"`
	// ClusterName is the name of the EKS cluster
	ClusterName string `json:"cluster-name"`
	// DevelopMode mode
	DevelopMode bool `json:"develop-mode"`
	// Weight Model

}

func LoadConfig(c *cli.Context) Config {
	var cfg Config
	cfg.KubeConfigPath = c.String("kubeconfig")
	cfg.ClusterName = c.String("cluster-name")
	cfg.DevelopMode = c.Bool("develop-mode")
	return cfg
}
