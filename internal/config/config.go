package config

import (
	"time"

	"github.com/urfave/cli/v2"
)

type Config struct {
	CompletionTaintKey string        `json:"completion-taint-key"`
	ReconcileInterval  time.Duration `json:"reconcile-interval"`
	MetricsAddress     string        `json:"metrics-address"`
	// AWSIPAMPoolID enables allocation from an AWS public IPv4 IPAM pool.
	AWSIPAMPoolID string `json:"aws-ipam-pool-id"`
	// LabelAssignedAddress publishes the assigned address in the legacy Node label.
	LabelAssignedAddress bool `json:"label-assigned-address"`
	// WaitForNodeAddress waits for Node status before labeling or removing the assignment taint.
	WaitForNodeAddress bool `json:"wait-for-node-address"`
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
	cfg.CompletionTaintKey = c.String("completion-taint-key")
	cfg.ReconcileInterval = c.Duration("reconcile-interval")
	cfg.MetricsAddress = c.String("metrics-address")
	cfg.AWSIPAMPoolID = c.String("aws-ipam-pool-id")
	cfg.LabelAssignedAddress = c.Bool("label-assigned-address")
	cfg.WaitForNodeAddress = c.Bool("wait-for-node-address")
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
