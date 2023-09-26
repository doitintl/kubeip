package types

import "net"

type CloudProvider string

const (
	CloudProviderGCP   CloudProvider = "gcp"
	CloudProviderAWS   CloudProvider = "aws"
	CloudProviderAzure CloudProvider = "azure"
)

type Node struct {
	Name        string
	Cloud       CloudProvider
	Pool        string
	Region      string
	Zone        string
	ExternalIPs []net.IP
	InternalIPs []net.IP
}
