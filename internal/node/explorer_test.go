package node

import (
	"context"
	"net"
	"os"
	"reflect"
	"testing"

	"github.com/doitintl/kubeip/internal/types"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

func matchErr(err1 error, err2 error) bool {
	errStr1 := ""
	errStr2 := ""
	if err1 != nil {
		errStr1 = err1.Error()
	}
	if err2 != nil {
		errStr2 = err2.Error()
	}
	return errStr1 == errStr2
}

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
		wantErr error
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
			name: "oci",
			args: args{
				providerID: "ocid1.instance.oc1.ap-mumbai-1.anrg6ljrdgsxvfacnncnwaxaasbdnjdgwuejhkbdfejkenoernoered",
			},
			want: types.CloudProviderOCI,
		},
		{
			name: "unsupported",
			args: args{
				providerID: "unsupported",
			},
			wantErr: errors.New("unsupported provider ID: unsupported"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getCloudProvider(tt.args.providerID)
			if !matchErr(err, tt.wantErr) {
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
		node       *v1.Node
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr error
	}{
		{
			name:    "nil node",
			wantErr: errors.New("node info is nil"),
		},
		{
			name: "oci",
			args: args{
				providerID: types.CloudProviderOCI,
				node: &v1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"oci.oraclecloud.com/node-pool-id": "ocid1.nodepool.oc1.ap-mumbai-1.aaaaaaaa7yv75wqblfix5rxnylajo35y3wabren",
						},
					},
				},
			},
			want: "ocid1.nodepool.oc1.ap-mumbai-1.aaaaaaaa7yv75wqblfix5rxnylajo35y3wabren",
		},
		{
			name: "aws",
			args: args{
				providerID: types.CloudProviderAWS,
				node: &v1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"eks.amazonaws.com/nodegroup": "test-node-pool",
							"beta.kubernetes.io/os":       "linux",
						},
					},
				},
			},
			want: "test-node-pool",
		},
		{
			name: "azure",
			args: args{
				providerID: types.CloudProviderAzure,
				node: &v1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"node.kubernetes.io/instancegroup": "test-node-pool",
							"beta.kubernetes.io/os":            "linux",
						},
					},
				},
			},
			want: "test-node-pool",
		},
		{
			name: "gcp",
			args: args{
				providerID: types.CloudProviderGCP,
				node: &v1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"cloud.google.com/gke-nodepool": "test-node-pool",
							"beta.kubernetes.io/os":         "linux",
						},
					},
				},
			},
			want: "test-node-pool",
		},
		{
			name: "unsupported",
			args: args{
				providerID: "unsupported",
				node: &v1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"beta.kubernetes.io/os": "linux",
						},
					},
				},
			},
			wantErr: errors.New("unsupported cloud provider: unsupported"),
		},
		{
			name: "no node pool",
			args: args{
				providerID: types.CloudProviderAWS,
				node: &v1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"beta.kubernetes.io/os": "linux",
						},
					},
				},
			},
			wantErr: errors.New("failed to get node pool"),
		},
		{
			name: "no node pool oci",
			args: args{
				providerID: types.CloudProviderOCI,
				node: &v1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{},
					},
				},
			},
			wantErr: errors.New("failed to get node pool"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getNodePool(tt.args.providerID, tt.args.node)
			if !matchErr(err, tt.wantErr) {
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
		{
			name: "skip unsupported IP type",
			args: args{
				addresses: []v1.NodeAddress{
					{Type: v1.NodeHostName, Address: "test-node"},
					{Type: v1.NodeInternalDNS, Address: "test-node-internal"},
					{Type: v1.NodeExternalIP, Address: "132.10.10.1"},
				},
			},
			want: []net.IP{net.ParseIP("132.10.10.1")},
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

func Test_explorer_GetNode(t *testing.T) {
	type fields struct {
		client kubernetes.Interface
	}
	type args struct {
		nodeName string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *types.Node
		wantErr error
	}{
		{
			name:    "nil client",
			wantErr: errors.New("kubernetes client is nil"),
		},
		{
			name:    "empty nodename",
			fields:  fields{client: fake.NewSimpleClientset()},
			wantErr: errors.New("failed to get node name from downward API: failed to read /etc/podinfo/nodeName: open /etc/podinfo/nodeName: no such file or directory"),
		},
		{
			name: "failed to get node",
			fields: fields{
				client: fake.NewSimpleClientset(),
			},
			args: args{
				nodeName: "test-node",
			},
			wantErr: errors.New("failed to get kubernetes node: nodes \"test-node\" not found"),
		},
		{
			name: "get node",
			fields: fields{
				client: fake.NewSimpleClientset(&v1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-node",
						Labels: map[string]string{
							"eks.amazonaws.com/nodegroup":   "test-node-pool",
							"beta.kubernetes.io/os":         "linux",
							"topology.kubernetes.io/region": "us-west-2",
							"topology.kubernetes.io/zone":   "us-west-2b",
						},
					},
					Spec: v1.NodeSpec{
						ProviderID: "aws:///us-west-2b/i-06d71a5ffc05cc325",
					},
					Status: v1.NodeStatus{
						Addresses: []v1.NodeAddress{
							{Type: v1.NodeExternalIP, Address: "132.10.10.1"},
							{Type: v1.NodeInternalIP, Address: "10.10.0.1"},
						},
					},
				}),
			},
			args: args{
				nodeName: "test-node",
			},
			want: &types.Node{
				Name:     "test-node",
				Instance: "i-06d71a5ffc05cc325",
				Cloud:    types.CloudProviderAWS,
				Pool:     "test-node-pool",
				Region:   "us-west-2",
				Zone:     "us-west-2b",
				ExternalIPs: []net.IP{
					net.ParseIP("132.10.10.1"),
				},
				InternalIPs: []net.IP{
					net.ParseIP("10.10.0.1"),
				},
			},
		},
		{
			name: "get node oci",
			fields: fields{
				client: fake.NewSimpleClientset(&v1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-node",
						Annotations: map[string]string{
							"oci.oraclecloud.com/node-pool-id": "ocid1.nodepool.oc1.ap-mumbai-1.test",
						},
						Labels: map[string]string{
							"topology.kubernetes.io/region": "us-west-2",
							"topology.kubernetes.io/zone":   "us-west-2b",
						},
					},
					Spec: v1.NodeSpec{
						ProviderID: "ocid1.instance.oc1.ap-mumbai-1.test",
					},
					Status: v1.NodeStatus{
						Addresses: []v1.NodeAddress{
							{Type: v1.NodeExternalIP, Address: "132.10.10.1"},
							{Type: v1.NodeInternalIP, Address: "10.10.0.1"},
						},
					},
				}),
			},
			args: args{
				nodeName: "test-node",
			},
			want: &types.Node{
				Name:     "test-node",
				Instance: "ocid1.instance.oc1.ap-mumbai-1.test",
				Cloud:    types.CloudProviderOCI,
				Pool:     "ocid1.nodepool.oc1.ap-mumbai-1.test",
				Region:   "us-west-2",
				Zone:     "us-west-2b",
				ExternalIPs: []net.IP{
					net.ParseIP("132.10.10.1"),
				},
				InternalIPs: []net.IP{
					net.ParseIP("10.10.0.1"),
				},
			},
		},
		{
			name: "failed to get cloud provider",
			fields: fields{
				client: fake.NewSimpleClientset(&v1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-node",
					},
				}),
			},
			args: args{
				nodeName: "test-node",
			},
			wantErr: errors.New("failed to get cloud provider: unsupported provider ID: "),
		},
		{
			name: "failed to get region",
			fields: fields{
				client: fake.NewSimpleClientset(&v1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-node",
						Labels: map[string]string{
							"eks.amazonaws.com/nodegroup": "test-node-pool",
							"beta.kubernetes.io/os":       "linux",
						},
					},
					Spec: v1.NodeSpec{
						ProviderID: "aws:///us-west-2b/i-06d71a5ffc05cc325",
					},
				}),
			},
			args: args{
				nodeName: "test-node",
			},
			wantErr: errors.New("failed to get node region"),
		},
		{
			name: "failed to get zone",
			fields: fields{
				client: fake.NewSimpleClientset(&v1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-node",
						Labels: map[string]string{
							"eks.amazonaws.com/nodegroup":   "test-node-pool",
							"beta.kubernetes.io/os":         "linux",
							"topology.kubernetes.io/region": "asia-south1",
						},
					},
					Spec: v1.NodeSpec{
						ProviderID: "aws:///us-west-2b/i-06d71a5ffc05cc325",
					},
				}),
			},
			args: args{
				nodeName: "test-node",
			},
			wantErr: errors.New("failed to get node zone"),
		},
		{
			name: "failed to get node pool",
			fields: fields{
				client: fake.NewSimpleClientset(&v1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-node",
						Labels: map[string]string{
							"beta.kubernetes.io/os":         "linux",
							"topology.kubernetes.io/region": "us-west-2",
							"topology.kubernetes.io/zone":   "us-west-2b",
						},
					},
					Spec: v1.NodeSpec{
						ProviderID: "aws:///us-west-2b/i-06d71a5ffc05cc325",
					},
					Status: v1.NodeStatus{
						Addresses: []v1.NodeAddress{
							{Type: v1.NodeExternalIP, Address: "132.10.10.1"},
							{Type: v1.NodeInternalIP, Address: "10.10.0.1"},
						},
					},
				}),
			},
			args: args{
				nodeName: "test-node",
			},
			wantErr: errors.New("failed to get node pool: failed to get node pool"),
		},
		{
			name: "failed to get node addresses",
			fields: fields{
				client: fake.NewSimpleClientset(&v1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-node",
						Labels: map[string]string{
							"eks.amazonaws.com/nodegroup":   "test-node-pool",
							"beta.kubernetes.io/os":         "linux",
							"topology.kubernetes.io/region": "us-west-2",
							"topology.kubernetes.io/zone":   "us-west-2b",
						},
					},
					Spec: v1.NodeSpec{
						ProviderID: "aws:///us-west-2b/i-06d71a5ffc05cc325",
					},
					Status: v1.NodeStatus{
						Addresses: []v1.NodeAddress{
							{Type: v1.NodeExternalIP, Address: "132.10.10.1"},
							{Type: v1.NodeInternalIP, Address: "address"},
						},
					},
				}),
			},
			args: args{
				nodeName: "test-node",
			},
			wantErr: errors.New("failed to get node addresses: failed to parse IP address: address"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &explorer{
				client: tt.fields.client,
			}
			got, err := d.GetNode(context.Background(), tt.args.nodeName)
			if !matchErr(err, tt.wantErr) {
				t.Errorf("GetNode() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetNode() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_getInstance(t *testing.T) {
	type args struct {
		providerID string
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr error
	}{
		{
			name: "empty provider ID",
			args: args{
				providerID: "",
			},
			wantErr: errors.New("failed to get instance ID, provider ID is empty"),
		},
		{
			name: "oci",
			args: args{
				providerID: "ocid1.instance.oc1.ap-mumbai-1.anrg6ljrdgsxvfacnncnwaxaasbdnjdgwuejhkbdfejkenoernoered",
			},
			want: "ocid1.instance.oc1.ap-mumbai-1.anrg6ljrdgsxvfacnncnwaxaasbdnjdgwuejhkbdfejkenoernoered",
		},
		{
			name: "aws",
			args: args{
				providerID: "aws:///us-west-2b/i-06d71a5ffc05cc325",
			},
			want: "i-06d71a5ffc05cc325",
		},
		{
			name: "azure",
			args: args{
				providerID: "azure:///subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/rg/providers/Microsoft.Compute/virtualMachines/aks-agentpool-12345678-vmss_0",
			},
			want: "aks-agentpool-12345678-vmss_0",
		},
		{
			name: "gcp",
			args: args{
				providerID: "gce:///projects/123456789012/zones/us-west1-b/instances/gke-cluster-1-default-pool-12345678-0v0v",
			},
			want: "gke-cluster-1-default-pool-12345678-0v0v",
		},
		{
			name: "unsupported",
			args: args{
				providerID: "unsupported",
			},
			wantErr: errors.New("failed to get instance ID"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getInstance(tt.args.providerID)
			if !matchErr(err, tt.wantErr) {
				t.Errorf("getInstance() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("getInstance() got = %v, want %v", got, tt.want)
			}
		})
	}
}
