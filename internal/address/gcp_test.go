package address

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/doitintl/kubeip/internal/cloud"
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
					mockCall.EXPECT().Filter("(status=RESERVED) (addressType=EXTERNAL) (test-filter-1) (test-filter-2)").Return(mockCall)
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
					mockCall.EXPECT().Filter("(status=RESERVED) (addressType=EXTERNAL) (test-filter-1) (test-filter-2)").Return(mockCall)
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
					mock.EXPECT().DeleteAccessConfig("test-project", "", args.instance.Name, accessConfigName, networkInterfaceName).Return(&compute.Operation{Name: "test-operation", Status: "DONE"}, nil)
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
			if err := a.deleteInstanceAddress(tt.args.ctx, tt.args.instance, tt.args.zone); (err != nil) != tt.wantErr {
				t.Errorf("deleteInstanceAddress() error = %v, wantErr %v", err, tt.wantErr)
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
				listerFn: func(t *testing.T) cloud.Lister {
					mock := mocks.NewLister(t)
					mockCall := mocks.NewListCall(t)
					mock.EXPECT().List("test-project", "test-region").Return(mockCall)
					mockCall.EXPECT().Filter("(status=IN_USE) (addressType=EXTERNAL)").Return(mockCall).Once()
					mockCall.EXPECT().Do().Return(&compute.AddressList{
						Items: []*compute.Address{
							{Name: "test-address-1", Status: inUseStatus, Address: "100.0.0.1", NetworkTier: "PREMIUM", AddressType: "EXTERNAL", Users: []string{"self-link-test-instance-1"}},
							{Name: "test-address-2", Status: inUseStatus, Address: "100.0.0.2", NetworkTier: "PREMIUM", AddressType: "EXTERNAL", Users: []string{"self-link-test-instance-2"}},
						},
					}, nil).Once()
					mockCall.EXPECT().Filter("(status=RESERVED) (addressType=EXTERNAL) (test-filter-1) (test-filter-2)").Return(mockCall).Once()
					mockCall.EXPECT().OrderBy("test-order-by").Return(mockCall).Once()
					mockCall.EXPECT().Do().Return(&compute.AddressList{
						Items: []*compute.Address{
							{Name: "test-address-3", Status: reservedStatus, Address: "100.0.0.3", NetworkTier: "PREMIUM", AddressType: "EXTERNAL"},
							{Name: "test-address-4", Status: reservedStatus, Address: "100.0.0.4", NetworkTier: "PREMIUM", AddressType: "EXTERNAL"},
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
									{Name: "test-access-config", NatIP: "200.0.0.1", Type: accessConfigType, Kind: accessConfigKind},
								},
							},
						},
					}, nil)
					return mock
				},
				addressManagerFn: func(t *testing.T) cloud.AddressManager {
					mock := mocks.NewAddressManager(t)
					mock.EXPECT().DeleteAccessConfig("test-project", "test-zone", "test-instance-0", "test-access-config", "test-network-interface").Return(&compute.Operation{Name: "test-operation", Status: "DONE"}, nil)
					mock.EXPECT().AddAccessConfig("test-project", "test-zone", "test-instance-0", defaultNetworkInterface, &compute.AccessConfig{
						Name:  "test-address-3",
						Type:  accessConfigType,
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
			if err := a.Assign(tt.args.ctx, tt.args.instanceID, tt.args.zone, tt.args.filter, tt.args.orderBy); (err != nil) != tt.wantErr {
				t.Errorf("Assign() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
