package address

import (
	"context"
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/doitintl/kubeip/internal/cloud"
	mocks "github.com/doitintl/kubeip/mocks/cloud"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

func Test_sortAddressesByTag(t *testing.T) {
	type args struct {
		addresses []types.Address
		key       string
	}
	tests := []struct {
		name string
		args args
		want []types.Address
	}{
		{
			name: "Test case 1: Sort addresses by tag value",
			args: args{
				addresses: []types.Address{
					{
						Tags: []types.Tag{
							{
								Key:   aws.String("Name"),
								Value: aws.String("B"),
							},
						},
					},
					{
						Tags: []types.Tag{
							{
								Key:   aws.String("Name"),
								Value: aws.String("A"),
							},
						},
					},
				},
				key: "Name",
			},
			want: []types.Address{
				{
					Tags: []types.Tag{
						{
							Key:   aws.String("Name"),
							Value: aws.String("A"),
						},
					},
				},
				{
					Tags: []types.Tag{
						{
							Key:   aws.String("Name"),
							Value: aws.String("B"),
						},
					},
				},
			},
		},
		{
			name: "Test case 2: Addresses with no tags",
			args: args{
				addresses: []types.Address{
					{},
					{},
				},
				key: "Name",
			},
			want: []types.Address{
				{},
				{},
			},
		},
		{
			name: "Test case 3: Key not found in tags",
			args: args{
				addresses: []types.Address{
					{
						Tags: []types.Tag{
							{
								Key:   aws.String("Name"),
								Value: aws.String("B"),
							},
						},
					},
					{
						Tags: []types.Tag{
							{
								Key:   aws.String("Name"),
								Value: aws.String("A"),
							},
						},
					},
				},
				key: "NonExistentKey",
			},
			want: []types.Address{
				{
					Tags: []types.Tag{
						{
							Key:   aws.String("Name"),
							Value: aws.String("B"),
						},
					},
				},
				{
					Tags: []types.Tag{
						{
							Key:   aws.String("Name"),
							Value: aws.String("A"),
						},
					},
				},
			},
		},
		{
			name: "Test case 4: One address with tags, one without",
			args: args{
				addresses: []types.Address{
					{
						Tags: []types.Tag{
							{
								Key:   aws.String("Name"),
								Value: aws.String("B"),
							},
						},
					},
					{},
				},
				key: "Name",
			},
			want: []types.Address{
				{
					Tags: []types.Tag{
						{
							Key:   aws.String("Name"),
							Value: aws.String("B"),
						},
					},
				},
				{},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sortAddressesByTag(tt.args.addresses, tt.args.key)
			if !reflect.DeepEqual(tt.args.addresses, tt.want) {
				t.Errorf("sortAddressesByTag() = %v, want %v", tt.args.addresses, tt.want)
			}
		})
	}
}

func Test_sortAddressesByField(t *testing.T) {
	type args struct {
		addresses []types.Address
		sortBy    string
	}
	tests := []struct {
		name string
		args args
		want []types.Address
	}{
		{
			name: "Test case 1: Sort addresses by tag value",
			args: args{
				addresses: []types.Address{
					{
						Tags: []types.Tag{
							{
								Key:   aws.String("Name"),
								Value: aws.String("B"),
							},
						},
					},
					{
						Tags: []types.Tag{
							{
								Key:   aws.String("Name"),
								Value: aws.String("A"),
							},
						},
					},
				},
				sortBy: "Tag:Name",
			},
			want: []types.Address{
				{
					Tags: []types.Tag{
						{
							Key:   aws.String("Name"),
							Value: aws.String("A"),
						},
					},
				},
				{
					Tags: []types.Tag{
						{
							Key:   aws.String("Name"),
							Value: aws.String("B"),
						},
					},
				},
			},
		},
		{
			name: "Test case 2: Sort addresses by AllocationId",
			args: args{
				addresses: []types.Address{
					{
						AllocationId: aws.String("b"),
					},
					{
						AllocationId: aws.String("a"),
					},
				},
				sortBy: "AllocationId",
			},
			want: []types.Address{
				{
					AllocationId: aws.String("a"),
				},
				{
					AllocationId: aws.String("b"),
				},
			},
		},
		{
			name: "Test case 3: Sort addresses by PublicIp",
			args: args{
				addresses: []types.Address{
					{
						PublicIp: aws.String("192.168.1.2"),
					},
					{
						PublicIp: aws.String("192.168.1.1"),
					},
				},
				sortBy: "PublicIp",
			},
			want: []types.Address{
				{
					PublicIp: aws.String("192.168.1.1"),
				},
				{
					PublicIp: aws.String("192.168.1.2"),
				},
			},
		},
		{
			name: "Test case 4: Sort addresses by InstanceId",
			args: args{
				addresses: []types.Address{
					{
						InstanceId: aws.String("i-0abcd1234efgh5678"),
					},
					{
						InstanceId: aws.String("i-0abcd1234efgh5679"),
					},
				},
				sortBy: "InstanceId",
			},
			want: []types.Address{
				{
					InstanceId: aws.String("i-0abcd1234efgh5678"),
				},
				{
					InstanceId: aws.String("i-0abcd1234efgh5679"),
				},
			},
		},
		{
			name: "Test case 5: Sort addresses by Domain",
			args: args{
				addresses: []types.Address{
					{
						Domain: types.DomainTypeVpc,
					},
					{
						Domain: types.DomainTypeStandard,
					},
				},
				sortBy: "Domain",
			},
			want: []types.Address{
				{
					Domain: types.DomainTypeStandard,
				},
				{
					Domain: types.DomainTypeVpc,
				},
			},
		},
		{
			name: "Test case 6: Sort addresses by NetworkInterfaceId",
			args: args{
				addresses: []types.Address{
					{
						NetworkInterfaceId: aws.String("eni-0abcd1234efgh5679"),
					},
					{
						NetworkInterfaceId: aws.String("eni-0abcd1234efgh5678"),
					},
				},
				sortBy: "NetworkInterfaceId",
			},
			want: []types.Address{
				{
					NetworkInterfaceId: aws.String("eni-0abcd1234efgh5678"),
				},
				{
					NetworkInterfaceId: aws.String("eni-0abcd1234efgh5679"),
				},
			},
		},
		{
			name: "Test case 7: Sort addresses by NetworkInterfaceOwnerId",
			args: args{
				addresses: []types.Address{
					{
						NetworkInterfaceOwnerId: aws.String("123456789013"),
					},
					{
						NetworkInterfaceOwnerId: aws.String("123456789012"),
					},
				},
				sortBy: "NetworkInterfaceOwnerId",
			},
			want: []types.Address{
				{
					NetworkInterfaceOwnerId: aws.String("123456789012"),
				},
				{
					NetworkInterfaceOwnerId: aws.String("123456789013"),
				},
			},
		},
		{
			name: "Test case 8: Sort addresses by AssociationId",
			args: args{
				addresses: []types.Address{
					{
						AssociationId: aws.String("b"),
					},
					{
						AssociationId: aws.String("a"),
					},
				},
				sortBy: "AssociationId",
			},
			want: []types.Address{
				{
					AssociationId: aws.String("a"),
				},
				{
					AssociationId: aws.String("b"),
				},
			},
		},
		{
			name: "Test case 9: Sort addresses by PrivateIpAddress",
			args: args{
				addresses: []types.Address{
					{
						PrivateIpAddress: aws.String("10.10.0.3"),
					},
					{
						PrivateIpAddress: aws.String("10.10.0.1"),
					},
				},
				sortBy: "PrivateIpAddress",
			},
			want: []types.Address{
				{
					PrivateIpAddress: aws.String("10.10.0.1"),
				},
				{
					PrivateIpAddress: aws.String("10.10.0.3"),
				},
			},
		},
		{
			name: "Test case 10: Sort addresses by PublicIpv4Pool",
			args: args{
				addresses: []types.Address{
					{
						PublicIpv4Pool: aws.String("amazon"),
					},
					{
						PublicIpv4Pool: aws.String("aws"),
					},
				},
				sortBy: "PublicIpv4Pool",
			},
			want: []types.Address{
				{
					PublicIpv4Pool: aws.String("amazon"),
				},
				{
					PublicIpv4Pool: aws.String("aws"),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sortAddressesByField(tt.args.addresses, tt.args.sortBy)
			if !reflect.DeepEqual(tt.args.addresses, tt.want) {
				t.Errorf("sortAddressesByField() = %v, want %v", tt.args.addresses, tt.want)
			}
		})
	}
}

func Test_awsAssigner_Assign(t *testing.T) {
	type args struct {
		ctx        context.Context
		instanceID string
		filter     []string
		orderBy    string
	}
	type fields struct {
		region           string
		logger           *logrus.Entry
		address          string
		instanceGetterFn func(t *testing.T, args *args) cloud.Ec2InstanceGetter
		eipListerFn      func(t *testing.T, args *args) cloud.EipLister
		eipAssignerFn    func(t *testing.T, args *args) cloud.EipAssigner
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		{
			name: "assign EIP to instance",
			fields: fields{
				region:  "us-east-1",
				logger:  logrus.NewEntry(logrus.New()),
				address: "100.0.0.1",
				instanceGetterFn: func(t *testing.T, args *args) cloud.Ec2InstanceGetter {
					mock := mocks.NewEc2InstanceGetter(t)
					mock.EXPECT().Get(args.ctx, args.instanceID, "us-east-1").Return(&types.Instance{
						InstanceId: aws.String(args.instanceID),
						NetworkInterfaces: []types.InstanceNetworkInterface{
							{
								Association: &types.InstanceNetworkInterfaceAssociation{
									PublicIp: aws.String("135.64.10.1"),
								},
								Attachment: &types.InstanceNetworkInterfaceAttachment{
									DeviceIndex: aws.Int32(0),
								},
								NetworkInterfaceId: aws.String("eni-0abcd1234efgh5678"),
							},
						},
					}, nil)
					return mock
				},
				eipListerFn: func(t *testing.T, args *args) cloud.EipLister {
					mock := mocks.NewEipLister(t)
					mock.EXPECT().List(args.ctx, map[string][]string{
						"instance-id": {args.instanceID},
					}, true).Return([]types.Address{}, nil).Once()
					mock.EXPECT().List(args.ctx, map[string][]string{
						"tag:env":    {"test"},
						"tag:kubeip": {"reserved"},
					}, false).Return([]types.Address{
						{
							AllocationId: aws.String("eipalloc-0abcd1234efgh5678"),
							PublicIp:     aws.String("100.0.0.1"),
							Tags: []types.Tag{
								{
									Key:   aws.String("env"),
									Value: aws.String("test"),
								},
								{
									Key:   aws.String("kubeip"),
									Value: aws.String("reserved"),
								},
							},
						},
						{
							AllocationId: aws.String("eipalloc-0abcd1234efgh5679"),
							PublicIp:     aws.String("100.0.0.2"),
							Tags: []types.Tag{
								{
									Key:   aws.String("env"),
									Value: aws.String("test"),
								},
								{
									Key:   aws.String("kubeip"),
									Value: aws.String("reserved"),
								},
							},
						},
					}, nil).Once()
					mock.EXPECT().List(args.ctx, map[string][]string{
						"allocation-id": {"eipalloc-0abcd1234efgh5678"},
					}, true).Return([]types.Address{
						{
							AllocationId: aws.String("eipalloc-0abcd1234efgh5678"),
							PublicIp:     aws.String("100.0.0.1"),
							Tags: []types.Tag{
								{
									Key:   aws.String("env"),
									Value: aws.String("test"),
								},
								{
									Key:   aws.String("kubeip"),
									Value: aws.String("reserved"),
								},
							},
						},
					}, nil).Once()
					return mock
				},
				eipAssignerFn: func(t *testing.T, args *args) cloud.EipAssigner {
					mock := mocks.NewEipAssigner(t)
					mock.EXPECT().Assign(args.ctx, "eni-0abcd1234efgh5678", "eipalloc-0abcd1234efgh5678").Return(nil)
					return mock
				},
			},
			args: args{
				ctx:        context.Background(),
				instanceID: "i-0abcd1234efgh5678",
				filter: []string{
					"Name=tag:env,Values=test",
					"Name=tag:kubeip,Values=reserved",
				},
				orderBy: "PublicIp",
			},
		},
		{
			name: "instance already has EIP assigned",
			fields: fields{
				region: "us-east-1",
				logger: logrus.NewEntry(logrus.New()),
				instanceGetterFn: func(t *testing.T, args *args) cloud.Ec2InstanceGetter {
					return nil
				},
				eipAssignerFn: func(t *testing.T, args *args) cloud.EipAssigner {
					return nil
				},
				eipListerFn: func(t *testing.T, args *args) cloud.EipLister {
					mock := mocks.NewEipLister(t)
					mock.EXPECT().List(args.ctx, map[string][]string{
						"instance-id": {args.instanceID},
					}, true).Return([]types.Address{
						{
							AllocationId: aws.String("eipalloc-0abcd1234efgh5678"),
							PublicIp:     aws.String("100.0.0.1"),
							Tags: []types.Tag{
								{
									Key:   aws.String("env"),
									Value: aws.String("test"),
								},
							},
						},
					}, nil)
					return mock
				},
			},
			args: args{
				ctx:        context.Background(),
				instanceID: "i-0abcd1234efgh5678",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &awsAssigner{
				region:         tt.fields.region,
				logger:         tt.fields.logger,
				instanceGetter: tt.fields.instanceGetterFn(t, &tt.args),
				eipLister:      tt.fields.eipListerFn(t, &tt.args),
				eipAssigner:    tt.fields.eipAssignerFn(t, &tt.args),
			}
			address, err := a.Assign(tt.args.ctx, tt.args.instanceID, "", tt.args.filter, tt.args.orderBy)
			if err != nil != tt.wantErr {
				t.Errorf("Assign() error = %v, wantErr %v", err, tt.wantErr)
			} else if address != tt.fields.address {
				t.Fatalf("Assign() = %v, want %v", address, tt.fields.address)
			}
		})
	}
}

func Test_parseShorthandFilter(t *testing.T) {
	type args struct {
		filter string
	}
	tests := []struct {
		name    string
		args    args
		want    string
		want1   []string
		wantErr bool
	}{
		{
			name: "parse shorthand filter",
			args: args{
				filter: "Name=tag:env,Values=val1,val2,val3",
			},
			want:  "tag:env",
			want1: []string{"val1", "val2", "val3"},
		},
		{
			name: "missing values",
			args: args{
				filter: "Name=tag:env",
			},
			wantErr: true,
		},
		{
			name: "bad filter",
			args: args{
				filter: "Name=tag:env,Values=val1,val2,val3;Kind=tag",
			},
			wantErr: true,
		},
		{
			name: "No name",
			args: args{
				filter: "Kind:tag:env,Values=val1,val2,val3",
			},
			wantErr: true,
		},
		{
			name: "No values",
			args: args{
				filter: "Name=tag:env,Kind=val1,val2,val3",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1, err := parseShorthandFilter(tt.args.filter)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseShorthandFilter() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("parseShorthandFilter() got = %v, want %v", got, tt.want)
			}
			if !reflect.DeepEqual(got1, tt.want1) {
				t.Errorf("parseShorthandFilter() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}

func Test_awsAssigner_tryAssignAddress(t *testing.T) {
	type args struct {
		address            *types.Address
		networkInterfaceID string
		instanceID         string
	}
	type fields struct {
		region        string
		eipListerFn   func(t *testing.T, args *args) cloud.EipLister
		eipAssignerFn func(t *testing.T, args *args) cloud.EipAssigner
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		{
			name: "assign EIP to instance",
			args: args{
				address: &types.Address{
					AllocationId: aws.String("eipalloc-0abcd1234efgh5678"),
					PublicIp:     aws.String("100.0.0.1"),
				},
				networkInterfaceID: "eni-0abcd1234efgh5678",
				instanceID:         "i-0abcd1234efgh5678",
			},
			fields: fields{
				region: "us-east-1",
				eipListerFn: func(t *testing.T, args *args) cloud.EipLister {
					mock := mocks.NewEipLister(t)
					mock.EXPECT().List(context.TODO(), map[string][]string{
						"allocation-id": {*args.address.AllocationId},
					}, true).Return([]types.Address{*args.address}, nil).Once()
					return mock
				},
				eipAssignerFn: func(t *testing.T, args *args) cloud.EipAssigner {
					mock := mocks.NewEipAssigner(t)
					mock.EXPECT().Assign(context.TODO(), args.networkInterfaceID, *args.address.AllocationId).Return(nil)
					return mock
				},
			},
		},
		{
			name: "EIP already assigned to instance",
			args: args{
				address: &types.Address{
					AllocationId:  aws.String("eipalloc-0abcd1234efgh5678"),
					PublicIp:      aws.String("100.0.0.1"),
					AssociationId: aws.String("eipassoc-0abcd1234efgh5678"),
				},
				networkInterfaceID: "eni-0abcd1234efgh5678",
				instanceID:         "i-0abcd1234efgh5678",
			},
			fields: fields{
				region: "us-east-1",
				eipListerFn: func(t *testing.T, args *args) cloud.EipLister {
					mock := mocks.NewEipLister(t)
					mock.EXPECT().List(context.TODO(), map[string][]string{
						"allocation-id": {*args.address.AllocationId},
					}, true).Return([]types.Address{*args.address}, nil).Once()
					return mock
				},
				eipAssignerFn: func(t *testing.T, args *args) cloud.EipAssigner {
					return nil
				},
			},
			wantErr: true,
		},
		{
			name: "EIP list error",
			args: args{
				address: &types.Address{
					AllocationId: aws.String("eipalloc-0abcd1234efgh5678"),
					PublicIp:     aws.String("100.0.0.1"),
				},
				networkInterfaceID: "eni-0abcd1234efgh5678",
				instanceID:         "i-0abcd1234efgh5678",
			},
			fields: fields{
				region: "us-east-1",
				eipListerFn: func(t *testing.T, args *args) cloud.EipLister {
					mock := mocks.NewEipLister(t)
					mock.EXPECT().List(context.TODO(), map[string][]string{
						"allocation-id": {*args.address.AllocationId},
					}, true).Return(nil, errors.New("test-error")).Once()
					return mock
				},
				eipAssignerFn: func(t *testing.T, args *args) cloud.EipAssigner {
					return nil
				},
			},
			wantErr: true,
		},
		{
			name: "EIP empty list",
			args: args{
				address: &types.Address{
					AllocationId: aws.String("eipalloc-0abcd1234efgh5678"),
					PublicIp:     aws.String("100.0.0.1"),
				},
				networkInterfaceID: "eni-0abcd1234efgh5678",
				instanceID:         "i-0abcd1234efgh5678",
			},
			fields: fields{
				region: "us-east-1",
				eipListerFn: func(t *testing.T, args *args) cloud.EipLister {
					mock := mocks.NewEipLister(t)
					mock.EXPECT().List(context.TODO(), map[string][]string{
						"allocation-id": {*args.address.AllocationId},
					}, true).Return([]types.Address{}, nil).Once()
					return mock
				},
				eipAssignerFn: func(t *testing.T, args *args) cloud.EipAssigner {
					mock := mocks.NewEipAssigner(t)
					mock.EXPECT().Assign(context.TODO(), args.networkInterfaceID, *args.address.AllocationId).Return(nil)
					return mock
				},
			},
		},
		{
			name: "EIP assign error",
			args: args{
				address: &types.Address{
					AllocationId: aws.String("eipalloc-0abcd1234efgh5678"),
					PublicIp:     aws.String("100.0.0.1"),
				},
				networkInterfaceID: "eni-0abcd1234efgh5678",
				instanceID:         "i-0abcd1234efgh5678",
			},
			fields: fields{
				region: "us-east-1",
				eipListerFn: func(t *testing.T, args *args) cloud.EipLister {
					mock := mocks.NewEipLister(t)
					mock.EXPECT().List(context.TODO(), map[string][]string{
						"allocation-id": {*args.address.AllocationId},
					}, true).Return([]types.Address{*args.address}, nil).Once()
					return mock
				},
				eipAssignerFn: func(t *testing.T, args *args) cloud.EipAssigner {
					mock := mocks.NewEipAssigner(t)
					mock.EXPECT().Assign(context.TODO(), args.networkInterfaceID, *args.address.AllocationId).Return(errors.New("test-error"))
					return mock
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &awsAssigner{
				region:      tt.fields.region,
				eipLister:   tt.fields.eipListerFn(t, &tt.args),
				eipAssigner: tt.fields.eipAssignerFn(t, &tt.args),
			}
			if err := a.tryAssignAddress(context.TODO(), tt.args.address, tt.args.networkInterfaceID, tt.args.instanceID); (err != nil) != tt.wantErr {
				t.Errorf("tryAssignAddress() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_awsAssigner_getNetworkInterfaceID(t *testing.T) {
	type args struct {
		instance *types.Instance
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "get network interface ID",
			args: args{
				instance: &types.Instance{
					NetworkInterfaces: []types.InstanceNetworkInterface{
						{
							Attachment: &types.InstanceNetworkInterfaceAttachment{
								DeviceIndex: aws.Int32(0),
							},
							Association: &types.InstanceNetworkInterfaceAssociation{
								PublicIp: aws.String("100.0.0.1"),
							},
							NetworkInterfaceId: aws.String("eni-0abcd1234efgh5678"),
						},
					},
				},
			},
			want: "eni-0abcd1234efgh5678",
		},
		{
			name: "no network interface ID",
			args: args{
				instance: &types.Instance{
					InstanceId:        aws.String("i-0abcd1234efgh5678"),
					NetworkInterfaces: []types.InstanceNetworkInterface{},
				},
			},
			wantErr: true,
		},
		{
			name: "no public IP",
			args: args{
				instance: &types.Instance{
					InstanceId: aws.String("i-0abcd1234efgh5678"),
					NetworkInterfaces: []types.InstanceNetworkInterface{
						{
							Attachment: &types.InstanceNetworkInterfaceAttachment{
								DeviceIndex: aws.Int32(0),
							},
							Association: &types.InstanceNetworkInterfaceAssociation{
								PublicIp: nil,
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "multiple network interfaces",
			args: args{
				instance: &types.Instance{
					NetworkInterfaces: []types.InstanceNetworkInterface{
						{
							Attachment: &types.InstanceNetworkInterfaceAttachment{
								DeviceIndex: aws.Int32(1),
							},
							Association: &types.InstanceNetworkInterfaceAssociation{
								PublicIp: aws.String("100.0.0.1"),
							},
							NetworkInterfaceId: aws.String("eni-0abcd1234efgh5678"),
						},
						{
							Attachment: &types.InstanceNetworkInterfaceAttachment{
								DeviceIndex: aws.Int32(0),
							},
							Association: &types.InstanceNetworkInterfaceAssociation{
								PublicIp: aws.String("100.0.0.2"),
							},
							NetworkInterfaceId: aws.String("eni-0abcd1234efgh5679"),
						},
					},
				},
			},
			want: "eni-0abcd1234efgh5679",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &awsAssigner{}
			got, err := a.getNetworkInterfaceID(tt.args.instance)
			if (err != nil) != tt.wantErr {
				t.Errorf("getNetworkInterfaceID() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("getNetworkInterfaceID() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_awsAssigner_getAssignedElasticIP(t *testing.T) {
	type args struct {
		instanceID string
	}
	type fields struct {
		region      string
		eipListerFn func(t *testing.T, args *args) cloud.EipLister
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *types.Address
		wantErr bool
	}{
		{
			name: "get assigned EIP",
			args: args{
				instanceID: "i-0abcd1234efgh5678",
			},
			fields: fields{
				region: "us-east-1",
				eipListerFn: func(t *testing.T, args *args) cloud.EipLister {
					mock := mocks.NewEipLister(t)
					mock.EXPECT().List(context.TODO(), map[string][]string{
						"instance-id": {args.instanceID},
					}, true).Return([]types.Address{
						{
							AllocationId: aws.String("eipalloc-0abcd1234efgh5678"),
							PublicIp:     aws.String("100.0.0.1"),
						},
					}, nil).Once()
					return mock
				},
			},
			want: &types.Address{
				AllocationId: aws.String("eipalloc-0abcd1234efgh5678"),
				PublicIp:     aws.String("100.0.0.1"),
			},
		},
		{
			name: "no assigned EIP",
			args: args{
				instanceID: "i-0abcd1234efgh5678",
			},
			fields: fields{
				region: "us-east-1",
				eipListerFn: func(t *testing.T, args *args) cloud.EipLister {
					mock := mocks.NewEipLister(t)
					mock.EXPECT().List(context.TODO(), map[string][]string{
						"instance-id": {args.instanceID},
					}, true).Return([]types.Address{}, nil).Once()
					return mock
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &awsAssigner{
				region:    tt.fields.region,
				eipLister: tt.fields.eipListerFn(t, &tt.args),
			}
			got, err := a.getAssignedElasticIP(context.TODO(), tt.args.instanceID)
			if (err != nil) != tt.wantErr {
				t.Errorf("getAssignedElasticIP() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getAssignedElasticIP() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_awsAssigner_Unassign(t *testing.T) {
	type args struct {
		instanceID string
	}
	type fields struct {
		region        string
		eipListerFn   func(t *testing.T, args *args) cloud.EipLister
		eipAssignerFn func(t *testing.T, args *args) cloud.EipAssigner
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		{
			name: "unassign EIP from instance",
			args: args{
				instanceID: "i-0abcd1234efgh5678",
			},
			fields: fields{
				region: "us-east-1",
				eipListerFn: func(t *testing.T, args *args) cloud.EipLister {
					mock := mocks.NewEipLister(t)
					mock.EXPECT().List(context.TODO(), map[string][]string{
						"instance-id": {args.instanceID},
					}, true).Return([]types.Address{
						{
							AllocationId:  aws.String("eipalloc-0abcd1234efgh5678"),
							AssociationId: aws.String("eipassoc-0abcd1234efgh5678"),
							PublicIp:      aws.String("100.0.0.1"),
						},
					}, nil).Once()
					return mock
				},
				eipAssignerFn: func(t *testing.T, args *args) cloud.EipAssigner {
					mock := mocks.NewEipAssigner(t)
					mock.EXPECT().Unassign(context.TODO(), "eipassoc-0abcd1234efgh5678").Return(nil)
					return mock
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &awsAssigner{
				region:      tt.fields.region,
				logger:      logrus.NewEntry(logrus.New()),
				eipLister:   tt.fields.eipListerFn(t, &tt.args),
				eipAssigner: tt.fields.eipAssignerFn(t, &tt.args),
			}
			if err := a.Unassign(context.TODO(), tt.args.instanceID, ""); (err != nil) != tt.wantErr {
				t.Errorf("Unassign() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
