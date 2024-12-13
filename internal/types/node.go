package types

import (
	"fmt"
	"net"
)

type CloudProvider string

const (
	CloudProviderGCP   CloudProvider = "gcp"
	CloudProviderAWS   CloudProvider = "aws"
	CloudProviderOCI   CloudProvider = "oci"
	CloudProviderAzure CloudProvider = "azure"
)

type Node struct {
	Name        string
	Instance    string
	Cloud       CloudProvider
	Pool        string
	Region      string
	Zone        string
	ExternalIPs []net.IP
	InternalIPs []net.IP
}

// Stringer interface: all fields with name and value
func (n *Node) String() string {
	return fmt.Sprintf("%+v", *n)
}
