package address

import (
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
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
