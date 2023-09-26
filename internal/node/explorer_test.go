package node

import (
	"net"
	"os"
	"reflect"
	"testing"

	"github.com/doitintl/kubeip/internal/types"
	v1 "k8s.io/api/core/v1"
)

func Test_getNodeName(t *testing.T) {
	tests := []struct {
		name     string
		nodeName string
		tearUp   func(name string) (*os.File, error)
		tearDown func(file string) error
		want     string
		wantErr  bool
	}{
		{
			name:     "get node name from .run/podinfo/nodeName file",
			nodeName: "test-node",
			tearUp: func(name string) (*os.File, error) {
				// Setup: create a temporary file and write some data to it
				tmpfile, err := os.CreateTemp("", "nodeName")
				if err != nil {
					return nil, err
				}
				if _, err = tmpfile.Write([]byte(name)); err != nil {
					return nil, err
				}
				if err = tmpfile.Close(); err != nil {
					return nil, err
				}
				return tmpfile, nil
			},
			tearDown: func(file string) error {
				return os.Remove(file)
			},
			want: "test-node",
		},
		{
			name:    "no such file or directory",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fileName := ""
			if tt.tearUp != nil {
				f, err := tt.tearUp(tt.nodeName)
				if err != nil {
					t.Fatal(err)
				}
				fileName = f.Name()
				defer func() {
					if tt.tearDown != nil {
						err = tt.tearDown(fileName)
						if err != nil {
							t.Fatal(err)
						}
					}
				}()
			}
			got, err := getNodeName(fileName)
			if (err != nil) != tt.wantErr {
				t.Errorf("getNodeName() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("getNodeName() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_getCloudProvider(t *testing.T) {
	type args struct {
		providerID string
	}
	tests := []struct {
		name    string
		args    args
		want    types.CloudProvider
		wantErr bool
	}{
		{
			name: "aws",
			args: args{
				providerID: "aws:///us-west-2b/i-06d71a5ffc05cc325",
			},
			want: types.CloudProviderAWS,
		},
		{
			name: "azure",
			args: args{
				providerID: "azure:///subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/rg/providers/Microsoft.Compute/virtualMachines/aks-agentpool-12345678-vmss_0",
			},
			want: types.CloudProviderAzure,
		},
		{
			name: "gcp",
			args: args{
				providerID: "gce:///projects/123456789012/zones/us-west1-b/instances/gke-cluster-1-default-pool-12345678-0v0v",
			},
			want: types.CloudProviderGCP,
		},
		{
			name: "unsupported",
			args: args{
				providerID: "unsupported",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getCloudProvider(tt.args.providerID)
			if (err != nil) != tt.wantErr {
				t.Errorf("getCloudProvider() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("getCloudProvider() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_getNodePool(t *testing.T) {
	type args struct {
		providerID types.CloudProvider
		labels     map[string]string
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "aws",
			args: args{
				providerID: types.CloudProviderAWS,
				labels: map[string]string{
					"eks.amazonaws.com/nodegroup": "test-node-pool",
					"beta.kubernetes.io/os":       "linux",
				},
			},
			want: "test-node-pool",
		},
		{
			name: "azure",
			args: args{
				providerID: types.CloudProviderAzure,
				labels: map[string]string{
					"node.kubernetes.io/instancegroup": "test-node-pool",
					"beta.kubernetes.io/os":            "linux",
				},
			},
			want: "test-node-pool",
		},
		{
			name: "gcp",
			args: args{
				providerID: types.CloudProviderGCP,
				labels: map[string]string{
					"cloud.google.com/gke-nodepool": "test-node-pool",
					"beta.kubernetes.io/os":         "linux",
				},
			},
			want: "test-node-pool",
		},
		{
			name: "unsupported",
			args: args{
				providerID: "unsupported",
				labels: map[string]string{
					"beta.kubernetes.io/os": "linux",
				},
			},
			wantErr: true,
		},
		{
			name: "no node pool",
			args: args{
				providerID: types.CloudProviderAWS,
				labels: map[string]string{
					"beta.kubernetes.io/os": "linux",
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getNodePool(tt.args.providerID, tt.args.labels)
			if (err != nil) != tt.wantErr {
				t.Errorf("getNodePool() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("getNodePool() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_getAddresses(t *testing.T) {
	type args struct {
		addresses []v1.NodeAddress
	}
	tests := []struct {
		name    string
		args    args
		want    []net.IP
		want1   []net.IP
		wantErr bool
	}{
		{
			name: "external and internal IPs",
			args: args{
				addresses: []v1.NodeAddress{
					{
						Type:    v1.NodeExternalIP,
						Address: "132.64.12.125",
					},
					{
						Type:    v1.NodeInternalIP,
						Address: "10.10.0.1",
					},
				},
			},
			want: []net.IP{
				net.ParseIP("132.64.12.125"),
			},
			want1: []net.IP{
				net.ParseIP("10.10.0.1"),
			},
		},
		{
			name: "no external IPs",
			args: args{
				addresses: []v1.NodeAddress{
					{Type: v1.NodeInternalIP, Address: "10.0.0.1"},
				},
			},
			want1: []net.IP{net.ParseIP("10.0.0.1")},
		},
		{
			name: "no internal IPs",
			args: args{
				addresses: []v1.NodeAddress{
					{Type: v1.NodeExternalIP, Address: "132.10.10.1"},
				},
			},
			want: []net.IP{net.ParseIP("132.10.10.1")},
		},
		{
			name: "invalid IP address",
			args: args{
				addresses: []v1.NodeAddress{
					{Type: v1.NodeExternalIP, Address: "invalid"},
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1, err := getAddresses(tt.args.addresses)
			if (err != nil) != tt.wantErr {
				t.Errorf("getAddresses() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getAddresses() got = %v, want %v", got, tt.want)
			}
			if !reflect.DeepEqual(got1, tt.want1) {
				t.Errorf("getAddresses() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}
