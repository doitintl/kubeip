package address

import (
	"reflect"
	"testing"

	"github.com/doitintl/kubeip/internal/cloud"
	mocks "github.com/doitintl/kubeip/mocks/cloud"
	"github.com/sirupsen/logrus"
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
