package address

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/doitintl/kubeip/internal/cloud"
	amock "github.com/doitintl/kubeip/mocks/address"
	mocks "github.com/doitintl/kubeip/mocks/cloud"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	tmock "github.com/stretchr/testify/mock"
	"google.golang.org/api/compute/v1"
)

func Test_gcpAssigner_listAddresses(t *testing.T) {
	type fields struct {
		listerFn func(t *testing.T) cloud.Lister
		project  string
		region   string
	}
	type args struct {
		filter  []string
		orderBy string
		status  string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    []*compute.Address
		wantErr bool
	}{
		{
			name: "list addresses successfully",
			fields: fields{
				project: "test-project",
				region:  "test-region",
				listerFn: func(t *testing.T) cloud.Lister {
					mock := mocks.NewLister(t)
					mockCall := mocks.NewListCall(t)
					mock.EXPECT().List("test-project", "test-region").Return(mockCall)
					mockCall.EXPECT().Filter("(status=RESERVED) (addressType=EXTERNAL) (ipVersion!=IPV6) (test-filter-1) (test-filter-2)").Return(mockCall)
					mockCall.EXPECT().OrderBy("test-order-by").Return(mockCall)
					mockCall.EXPECT().Do().Return(&compute.AddressList{
						Items: []*compute.Address{
							{Name: "test-address-1", Status: "RESERVED", Address: "10.10.0.1", NetworkTier: "PREMIUM", AddressType: "EXTERNAL"},
							{Name: "test-address-2", Status: "RESERVED", Address: "10.10.0.2", NetworkTier: "PREMIUM", AddressType: "EXTERNAL"},
						},
					}, nil)
					return mock
				},
			},
			args: args{
				filter:  []string{"test-filter-1", "test-filter-2"},
				orderBy: "test-order-by",
				status:  "RESERVED",
			},
			want: []*compute.Address{
				{Name: "test-address-1", Status: "RESERVED", Address: "10.10.0.1", NetworkTier: "PREMIUM", AddressType: "EXTERNAL"},
				{Name: "test-address-2", Status: "RESERVED", Address: "10.10.0.2", NetworkTier: "PREMIUM", AddressType: "EXTERNAL"},
			},
		},
		{
			name: "list addresses with multiple pages successfully",
			fields: fields{
				project: "test-project",
				region:  "test-region",
				listerFn: func(t *testing.T) cloud.Lister {
					mock := mocks.NewLister(t)
					mockCall := mocks.NewListCall(t)
					mock.EXPECT().List("test-project", "test-region").Return(mockCall)
					mockCall.EXPECT().Filter("(status=RESERVED) (addressType=EXTERNAL) (ipVersion!=IPV6) (test-filter-1) (test-filter-2)").Return(mockCall)
					mockCall.EXPECT().OrderBy("test-order-by").Return(mockCall)
					mockCall.EXPECT().Do().Return(&compute.AddressList{
						Items: []*compute.Address{
							{Name: "test-address-1", Status: "RESERVED", Address: "10.10.0.1", NetworkTier: "PREMIUM", AddressType: "EXTERNAL"},
							{Name: "test-address-2", Status: "RESERVED", Address: "10.10.0.2", NetworkTier: "PREMIUM", AddressType: "EXTERNAL"},
						},
						NextPageToken: "test-next-page-token",
					}, nil).Once()
					mockCall.EXPECT().PageToken("test-next-page-token").Return(mockCall)
					mockCall.EXPECT().Do().Return(&compute.AddressList{
						Items: []*compute.Address{
							{Name: "test-address-3", Status: "RESERVED", Address: "10.10.0.3", NetworkTier: "PREMIUM", AddressType: "EXTERNAL"},
							{Name: "test-address-4", Status: "RESERVED", Address: "10.10.0.4", NetworkTier: "PREMIUM", AddressType: "EXTERNAL"},
						},
					}, nil).Once()
					return mock
				},
			},
			args: args{
				filter:  []string{"test-filter-1", "test-filter-2"},
				orderBy: "test-order-by",
				status:  "RESERVED",
			},
			want: []*compute.Address{
				{Name: "test-address-1", Status: "RESERVED", Address: "10.10.0.1", NetworkTier: "PREMIUM", AddressType: "EXTERNAL"},
				{Name: "test-address-2", Status: "RESERVED", Address: "10.10.0.2", NetworkTier: "PREMIUM", AddressType: "EXTERNAL"},
				{Name: "test-address-3", Status: "RESERVED", Address: "10.10.0.3", NetworkTier: "PREMIUM", AddressType: "EXTERNAL"},
				{Name: "test-address-4", Status: "RESERVED", Address: "10.10.0.4", NetworkTier: "PREMIUM", AddressType: "EXTERNAL"},
			},
		},
	}
	for _, tt := range tests {
		logger := logrus.NewEntry(logrus.New())
		t.Run(tt.name, func(t *testing.T) {
			a := &gcpAssigner{
				lister:  tt.fields.listerFn(t),
				project: tt.fields.project,
				region:  tt.fields.region,
				logger:  logger,
			}
			got, err := a.listAddresses(tt.args.filter, tt.args.orderBy, tt.args.status)
			if (err != nil) != tt.wantErr {
				t.Errorf("listAddresses() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("listAddresses() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_gcpAssigner_waitForOperation(t *testing.T) {
	type fields struct {
		waiterFn func(t *testing.T) cloud.ZoneWaiter
		project  string
	}
	type args struct {
		op      *compute.Operation
		zone    string
		timeout time.Duration
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		{
			name: "wait for operation successfully",
			fields: fields{
				project: "test-project",
				waiterFn: func(t *testing.T) cloud.ZoneWaiter {
					mock := mocks.NewZoneWaiter(t)
					mockCall := mocks.NewWaitCall(t)
					mock.EXPECT().Wait("test-project", "test-zone", "test-operation").Return(mockCall)
					mockCall.EXPECT().Context(tmock.Anything).Return(mockCall)
					mockCall.EXPECT().Do().Return(&compute.Operation{Status: "DONE"}, nil)
					return mock
				},
			},
			args: args{
				op:      &compute.Operation{Name: "test-operation", Status: "RUNNING"},
				zone:    "test-zone",
				timeout: time.Millisecond,
			},
		},
		{
			name: "wait for operation with a few retries successfully",
			fields: fields{
				project: "test-project",
				waiterFn: func(t *testing.T) cloud.ZoneWaiter {
					mock := mocks.NewZoneWaiter(t)
					mockCall := mocks.NewWaitCall(t)
					mock.EXPECT().Wait("test-project", "test-zone", "test-operation").Return(mockCall)
					mockCall.EXPECT().Context(tmock.Anything).Return(mockCall)
					mockCall.EXPECT().Do().Return(&compute.Operation{Status: "RUNNING"}, nil).Times(2)
					mockCall.EXPECT().Do().Return(&compute.Operation{Status: "DONE"}, nil)
					return mock
				},
			},
			args: args{
				op:      &compute.Operation{Name: "test-operation", Status: "RUNNING"},
				zone:    "test-zone",
				timeout: time.Millisecond * 2,
			},
		},
		{
			name: "wait for operation with timeout",
			fields: fields{
				project: "test-project",
				waiterFn: func(t *testing.T) cloud.ZoneWaiter {
					mock := mocks.NewZoneWaiter(t)
					mockCall := mocks.NewWaitCall(t)
					mock.EXPECT().Wait("test-project", "test-zone", "test-operation").Return(mockCall)
					mockCall.EXPECT().Context(tmock.Anything).Return(mockCall)
					mockCall.EXPECT().Do().Return(nil, context.Canceled)
					return mock
				},
			},
			args: args{
				op:      &compute.Operation{Name: "test-operation", Status: "RUNNING"},
				zone:    "test-zone",
				timeout: time.Millisecond,
			},
			wantErr: true,
		},
		{
			name: "wait for operation with error",
			fields: fields{
				project: "test-project",
				waiterFn: func(t *testing.T) cloud.ZoneWaiter {
					mock := mocks.NewZoneWaiter(t)
					mockCall := mocks.NewWaitCall(t)
					mock.EXPECT().Wait("test-project", "test-zone", "test-operation").Return(mockCall)
					mockCall.EXPECT().Context(tmock.Anything).Return(mockCall)
					mockCall.EXPECT().Do().Return(nil, errors.New("test-error"))
					return mock
				},
			},
			args: args{
				op:      &compute.Operation{Name: "test-operation", Status: "RUNNING"},
				zone:    "test-zone",
				timeout: time.Millisecond,
			},
			wantErr: true,
		},
		{
			name: "wait for operation with error in operation",
			fields: fields{
				project: "test-project",
				waiterFn: func(t *testing.T) cloud.ZoneWaiter {
					mock := mocks.NewZoneWaiter(t)
					mockCall := mocks.NewWaitCall(t)
					mock.EXPECT().Wait("test-project", "test-zone", "test-operation").Return(mockCall)
					mockCall.EXPECT().Context(tmock.Anything).Return(mockCall)
					mockCall.EXPECT().Do().Return(&compute.Operation{Status: "DONE", Error: &compute.OperationError{Errors: []*compute.OperationErrorErrors{{Code: "123", Message: "test-error"}}}}, nil)
					return mock
				},
			},
			args: args{
				op:      &compute.Operation{Name: "test-operation", Status: "RUNNING"},
				zone:    "test-zone",
				timeout: time.Millisecond,
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := logrus.NewEntry(logrus.New())
			waiter := tt.fields.waiterFn(t)
			a := &gcpAssigner{
				waiter:  waiter,
				project: tt.fields.project,
				logger:  logger,
			}
			if err := a.waitForOperation(context.TODO(), tt.args.op, tt.args.zone, tt.args.timeout); (err != nil) != tt.wantErr {
				t.Errorf("waitForOperation() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_gcpAssigner_deleteInstanceAddress(t *testing.T) {
	type args struct {
		ctx      context.Context
		instance *compute.Instance
		zone     string
	}
	type fields struct {
		addressManagerFn func(t *testing.T, args *args) cloud.AddressManager
		project          string
		region           string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		{
			name: "delete instance address successfully",
			fields: fields{
				project: "test-project",
				region:  "test-region",
				addressManagerFn: func(t *testing.T, args *args) cloud.AddressManager {
					mock := mocks.NewAddressManager(t)
					networkInterfaceName := args.instance.NetworkInterfaces[0].Name
					accessConfigName := args.instance.NetworkInterfaces[0].AccessConfigs[0].Name
					fingerprint := args.instance.NetworkInterfaces[0].Fingerprint
					mock.EXPECT().DeleteAccessConfig("test-project", "", args.instance.Name, accessConfigName, networkInterfaceName, fingerprint).Return(&compute.Operation{Name: "test-operation", Status: "DONE"}, nil)
					return mock
				},
			},
			args: args{
				ctx: context.TODO(),
				instance: &compute.Instance{
					Name: "test-instance",
					Zone: "test-zone",
					NetworkInterfaces: []*compute.NetworkInterface{
						{
							Name: "test-network-interface",
							AccessConfigs: []*compute.AccessConfig{
								{Name: "test-access-config", NatIP: "100.0.0.1"},
							},
							Fingerprint: "test-fingerprint",
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := logrus.NewEntry(logrus.New())
			a := &gcpAssigner{
				addressManager: tt.fields.addressManagerFn(t, &tt.args),
				project:        tt.fields.project,
				region:         tt.fields.region,
				logger:         logger,
			}
			if err := a.DeleteInstanceAddress(tt.args.ctx, tt.args.instance, tt.args.zone); (err != nil) != tt.wantErr {
				t.Errorf("DeleteInstanceAddress() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_gcpAssigner_Assign(t *testing.T) {
	type fields struct {
		listerFn         func(t *testing.T) cloud.Lister
		addressManagerFn func(t *testing.T) cloud.AddressManager
		instanceGetterFn func(t *testing.T) cloud.InstanceGetter
		project          string
		region           string
		address          string
	}
	type args struct {
		ctx        context.Context
		instanceID string
		zone       string
		filter     []string
		orderBy    string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		{
			name: "assign static IP address successfully",
			fields: fields{
				project: "test-project",
				region:  "test-region",
				address: "100.0.0.3",
				listerFn: func(t *testing.T) cloud.Lister {
					mock := mocks.NewLister(t)
					mockCall := mocks.NewListCall(t)
					mock.EXPECT().List("test-project", "test-region").Return(mockCall)
					mockCall.EXPECT().Filter("(status=IN_USE) (addressType=EXTERNAL) (ipVersion!=IPV6)").Return(mockCall).Once()
					mockCall.EXPECT().Do().Return(&compute.AddressList{
						Items: []*compute.Address{
							{Name: "test-address-1", Status: inUseStatus, Address: "100.0.0.1", NetworkTier: defaultNetworkTier, AddressType: "EXTERNAL", Users: []string{"self-link-test-instance-1"}},
							{Name: "test-address-2", Status: inUseStatus, Address: "100.0.0.2", NetworkTier: defaultNetworkTier, AddressType: "EXTERNAL", Users: []string{"self-link-test-instance-2"}},
						},
					}, nil).Once()
					mockCall.EXPECT().Filter("(status=RESERVED) (addressType=EXTERNAL) (ipVersion!=IPV6) (test-filter-1) (test-filter-2)").Return(mockCall).Once()
					mockCall.EXPECT().OrderBy("test-order-by").Return(mockCall).Once()
					mockCall.EXPECT().Do().Return(&compute.AddressList{
						Items: []*compute.Address{
							{Name: "test-address-3", Status: reservedStatus, Address: "100.0.0.3", NetworkTier: defaultNetworkTier, AddressType: "EXTERNAL"},
							{Name: "test-address-4", Status: reservedStatus, Address: "100.0.0.4", NetworkTier: defaultNetworkTier, AddressType: "EXTERNAL"},
						},
					}, nil).Once()
					return mock
				},
				instanceGetterFn: func(t *testing.T) cloud.InstanceGetter {
					mock := mocks.NewInstanceGetter(t)
					mock.EXPECT().Get("test-project", "test-zone", "test-instance-0").Return(&compute.Instance{
						Name: "test-instance-0",
						Zone: "test-zone",
						NetworkInterfaces: []*compute.NetworkInterface{
							{
								Name: "test-network-interface",
								AccessConfigs: []*compute.AccessConfig{
									{Name: "test-access-config", NatIP: "200.0.0.1", Type: defaultAccessConfigType, Kind: accessConfigKind},
								},
								Fingerprint: "test-fingerprint",
							},
						},
					}, nil)
					return mock
				},
				addressManagerFn: func(t *testing.T) cloud.AddressManager {
					mock := mocks.NewAddressManager(t)
					mock.EXPECT().DeleteAccessConfig("test-project", "test-zone", "test-instance-0", "test-access-config", "test-network-interface", "test-fingerprint").Return(&compute.Operation{Name: "test-operation", Status: "DONE"}, nil)
					mock.EXPECT().AddAccessConfig("test-project", "test-zone", "test-instance-0", "test-network-interface", "test-fingerprint", &compute.AccessConfig{
						Name:  defaultNetworkName,
						Type:  defaultAccessConfigType,
						Kind:  accessConfigKind,
						NatIP: "100.0.0.3",
					}).Return(&compute.Operation{Name: "test-operation", Status: "DONE"}, nil)
					mock.EXPECT().GetAddress("test-project", "test-region", "test-address-3").Return(&compute.Address{Name: "test-address-3", Status: reservedStatus}, nil)
					return mock
				},
			},
			args: args{
				ctx:        context.TODO(),
				instanceID: "test-instance-0",
				zone:       "test-zone",
				filter:     []string{"test-filter-1", "test-filter-2"},
				orderBy:    "test-order-by",
			},
		},
		{
			name: "assign when static IP address already allocted",
			fields: fields{
				project: "test-project",
				region:  "test-region",
				address: "100.0.0.2",
				listerFn: func(t *testing.T) cloud.Lister {
					mock := mocks.NewLister(t)
					mockCall := mocks.NewListCall(t)
					mock.EXPECT().List("test-project", "test-region").Return(mockCall)
					mockCall.EXPECT().Filter("(status=IN_USE) (addressType=EXTERNAL) (ipVersion!=IPV6)").Return(mockCall).Once()
					mockCall.EXPECT().Do().Return(&compute.AddressList{
						Items: []*compute.Address{
							{Name: "test-address-1", Status: inUseStatus, Address: "100.0.0.1", NetworkTier: defaultNetworkTier, AddressType: "EXTERNAL", Users: []string{"self-link-test-instance-1"}},
							{Name: "test-address-2", Status: inUseStatus, Address: "100.0.0.2", NetworkTier: defaultNetworkTier, AddressType: "EXTERNAL", Users: []string{"self-link-test-instance-2"}},
						},
					}, nil).Once()
					return mock
				},
				instanceGetterFn: func(t *testing.T) cloud.InstanceGetter {
					mock := mocks.NewInstanceGetter(t)
					mock.EXPECT().Get("test-project", "test-zone", "test-instance-0").Return(&compute.Instance{
						Name:     "test-instance-0",
						Zone:     "test-zone",
						SelfLink: "self-link-test-instance-2",
						NetworkInterfaces: []*compute.NetworkInterface{
							{
								Name: "test-network-interface",
								AccessConfigs: []*compute.AccessConfig{
									{Name: "test-access-config", NatIP: "200.0.0.1", Type: defaultAccessConfigType, Kind: accessConfigKind},
								},
								Fingerprint: "test-fingerprint",
							},
						},
					}, nil)
					return mock
				},
				addressManagerFn: func(t *testing.T) cloud.AddressManager {
					return mocks.NewAddressManager(t)
				},
			},
			args: args{
				ctx:        context.TODO(),
				instanceID: "test-instance-0",
				zone:       "test-zone",
				filter:     []string{"test-filter-1", "test-filter-2"},
				orderBy:    "test-order-by",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := logrus.NewEntry(logrus.New())
			a := &gcpAssigner{
				lister:         tt.fields.listerFn(t),
				addressManager: tt.fields.addressManagerFn(t),
				instanceGetter: tt.fields.instanceGetterFn(t),
				project:        tt.fields.project,
				region:         tt.fields.region,
				logger:         logger,
			}
			address, err := a.Assign(tt.args.ctx, tt.args.instanceID, tt.args.zone, tt.args.filter, tt.args.orderBy)
			if err != nil != tt.wantErr {
				t.Errorf("Assign() error = %v, wantErr %v", err, tt.wantErr)
			} else if address != tt.fields.address {
				t.Fatalf("Assign() = %v, want %v", address, tt.fields.address)
			}
		})
	}
}

func Test_createAccessConfig(t *testing.T) {
	type args struct {
		address *compute.Address
		ipv6    bool
	}
	tests := []struct {
		name string
		args args
		want *compute.AccessConfig
	}{
		{
			name: "create access config for IPv4 address",
			args: args{
				address: &compute.Address{
					Name:    "test-address",
					Address: "100.0.0.1",
				},
			},
			want: &compute.AccessConfig{
				Name:  defaultNetworkName,
				Type:  defaultAccessConfigType,
				Kind:  accessConfigKind,
				NatIP: "100.0.0.1",
			},
		},
		{
			name: "create access config for IPv6 address",
			args: args{
				address: &compute.Address{
					Name:         "test-address",
					Address:      "2001:db8::1",
					PrefixLength: 128,
					NetworkTier:  "TEST",
				},
				ipv6: true,
			},
			want: &compute.AccessConfig{
				Name:                     defaultNetworkNameIPv6,
				Type:                     defaultAccessConfigIPv6Type,
				Kind:                     accessConfigKind,
				ExternalIpv6:             "2001:db8::1",
				NetworkTier:              "TEST",
				ExternalIpv6PrefixLength: 128,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := createAccessConfig(tt.args.address, tt.args.ipv6); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("createAccessConfig() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_getNetworkInterface(t *testing.T) {
	type args struct {
		instance *compute.Instance
	}
	tests := []struct {
		name    string
		args    args
		want    *compute.NetworkInterface
		wantErr bool
	}{
		{
			name: "get network interface successfully",
			args: args{
				instance: &compute.Instance{
					Name: "test-instance",
					NetworkInterfaces: []*compute.NetworkInterface{
						{Name: "test-network-interface"},
					},
				},
			},
			want: &compute.NetworkInterface{Name: "test-network-interface"},
		},
		{
			name: "get network interface with error",
			args: args{
				instance: &compute.Instance{
					Name: "test-instance",
				},
			},
			wantErr: true,
		},
		{
			name: "get first network interface successfully",
			args: args{
				instance: &compute.Instance{
					Name: "test-instance",
					NetworkInterfaces: []*compute.NetworkInterface{
						{Name: "test-network-interface-1"},
						{Name: "test-network-interface-2"},
					},
				},
			},
			want: &compute.NetworkInterface{Name: "test-network-interface-1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getNetworkInterface(tt.args.instance)
			if (err != nil) != tt.wantErr {
				t.Errorf("getNetworkInterface() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getNetworkInterface() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_getAccessConfig(t *testing.T) {
	type args struct {
		networkInterface *compute.NetworkInterface
		ipv6             bool
	}
	tests := []struct {
		name    string
		args    args
		want    *compute.AccessConfig
		wantErr bool
	}{
		{
			name: "get access config successfully",
			args: args{
				networkInterface: &compute.NetworkInterface{
					Name: "test-network-interface",
					AccessConfigs: []*compute.AccessConfig{
						{Name: "test-access-config"},
					},
				},
			},
			want: &compute.AccessConfig{Name: "test-access-config"},
		},
		{
			name: "get access config ipv6 successfully",
			args: args{
				networkInterface: &compute.NetworkInterface{
					Name: "test-network-interface",
					Ipv6AccessConfigs: []*compute.AccessConfig{
						{Name: "test-access-config"},
					},
				},
				ipv6: true,
			},
			want: &compute.AccessConfig{Name: "test-access-config"},
		},
		{
			name: "no network interface error",
			args: args{
				networkInterface: &compute.NetworkInterface{},
			},
			wantErr: true,
		},
		{
			name: "no access config error",
			args: args{
				networkInterface: &compute.NetworkInterface{
					Name: "test-network-interface",
				},
			},
			wantErr: true,
		},
		{
			name: "no access config ipv6 error",
			args: args{
				networkInterface: &compute.NetworkInterface{
					Name: "test-network-interface",
				},
				ipv6: true,
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getAccessConfig(tt.args.networkInterface, tt.args.ipv6)
			if (err != nil) != tt.wantErr {
				t.Errorf("getAccessConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getAccessConfig() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_retryAddEphemeralAddress(t *testing.T) {
	type args struct {
		asFn     func(t *testing.T) internalAssigner
		instance *compute.Instance
		zone     string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "retry add ephemeral address successfully",
			args: args{
				zone: "test-zone",
				asFn: func(t *testing.T) internalAssigner {
					mock := amock.NewInternalAssigner(t)
					mock.EXPECT().AddInstanceAddress(context.TODO(), tmock.Anything, "test-zone", tmock.Anything).Return(nil)
					return mock
				},
			},
		},
		{
			name: "retry add ephemeral address with error",
			args: args{
				zone: "test-zone",
				asFn: func(t *testing.T) internalAssigner {
					mock := amock.NewInternalAssigner(t)
					mock.EXPECT().AddInstanceAddress(context.TODO(), tmock.Anything, "test-zone", tmock.Anything).Return(errors.New("test-error")).Times(3)
					mock.EXPECT().AddInstanceAddress(context.TODO(), tmock.Anything, "test-zone", tmock.Anything).Return(nil)
					return mock
				},
			},
		},
		{
			name: "retry add ephemeral address with error and max retries reached",
			args: args{
				zone: "test-zone",
				asFn: func(t *testing.T) internalAssigner {
					mock := amock.NewInternalAssigner(t)
					mock.EXPECT().AddInstanceAddress(context.TODO(), tmock.Anything, "test-zone", tmock.Anything).Return(errors.New("test-error")).Times(maxRetries)
					return mock
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := retryAddEphemeralAddress(context.TODO(), logrus.NewEntry(logrus.New()), tt.args.asFn(t), tt.args.instance, tt.args.zone); (err != nil) != tt.wantErr {
				t.Errorf("retryAddEphemeralAddress() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_tryAssignAddress(t *testing.T) {
	type args struct {
		asFn     func(t *testing.T) internalAssigner
		instance *compute.Instance
		region   string
		zone     string
		address  *compute.Address
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "try assign address successfully",
			args: args{
				zone:   "test-zone",
				region: "test-region",
				asFn: func(t *testing.T) internalAssigner {
					mock := amock.NewInternalAssigner(t)
					mock.EXPECT().CheckAddressAssigned("test-region", "test-address").Return(false, nil)
					mock.EXPECT().AddInstanceAddress(context.TODO(), tmock.Anything, "test-zone", tmock.Anything).Return(nil)
					return mock
				},
				address: &compute.Address{
					Name: "test-address",
				},
			},
		},
		{
			name: "try assign address already assigned",
			args: args{
				zone:   "test-zone",
				region: "test-region",
				asFn: func(t *testing.T) internalAssigner {
					mock := amock.NewInternalAssigner(t)
					mock.EXPECT().CheckAddressAssigned("test-region", "test-address").Return(true, nil)
					return mock
				},
				address: &compute.Address{
					Name: "test-address",
				},
			},
			wantErr: true,
		},
		{
			name: "try assign address with check error",
			args: args{
				zone:   "test-zone",
				region: "test-region",
				asFn: func(t *testing.T) internalAssigner {
					mock := amock.NewInternalAssigner(t)
					mock.EXPECT().CheckAddressAssigned("test-region", "test-address").Return(false, errors.New("test-error"))
					return mock
				},
				address: &compute.Address{
					Name: "test-address",
				},
			},
			wantErr: true,
		},
		{
			name: "try assign address with add assign error",
			args: args{
				zone:   "test-zone",
				region: "test-region",
				asFn: func(t *testing.T) internalAssigner {
					mock := amock.NewInternalAssigner(t)
					mock.EXPECT().CheckAddressAssigned("test-region", "test-address").Return(false, nil)
					mock.EXPECT().AddInstanceAddress(context.TODO(), tmock.Anything, "test-zone", tmock.Anything).Return(errors.New("test-error"))
					return mock
				},
				address: &compute.Address{
					Name: "test-address",
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tryAssignAddress(context.TODO(), tt.args.asFn(t), tt.args.instance, tt.args.region, tt.args.zone, tt.args.address); (err != nil) != tt.wantErr {
				t.Errorf("tryAssignAddress() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
