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
	// Project is the name of the GCP project or the AWS account ID or the OCI compartment OCID
	Project string `json:"project"`
	// Region is the name of the GCP region or the AWS region or the OCI region
	Region string `json:"region"`
	// IPv6 support
	IPv6 bool `json:"ipv6"`
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
	// ReleaseOnExit releases the IP address on exit
	ReleaseOnExit bool `json:"release-on-exit"`
	// LeaseDuration is the duration of the kubernetes lease
	LeaseDuration int `json:"lease-duration"`
	// LeaseNamespace is the namespace of the kubernetes lease
	LeaseNamespace string `json:"lease-namespace"`
	// TaintKey is the taint key to remove from the node once the IP address is assigned
	TaintKey string `json:"taint-key"`
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
	cfg.IPv6 = c.Bool("ipv6")
	cfg.ReleaseOnExit = c.Bool("release-on-exit")
	cfg.LeaseDuration = c.Int("lease-duration")
	cfg.LeaseNamespace = c.String("lease-namespace")
	cfg.TaintKey = c.String("taint-key")
	return &cfg
}
