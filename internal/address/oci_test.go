package address

import (
	"context"
	"reflect"
	"testing"

	"github.com/doitintl/kubeip/internal/cloud"
	"github.com/doitintl/kubeip/internal/config"
	"github.com/doitintl/kubeip/internal/types"
	cmocks "github.com/doitintl/kubeip/mocks/cloud"
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/mock"
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

func Test_ociAssigner_Assign(t *testing.T) {
	type args struct {
		compartmentOCID string
		instanceOCID    string
		filters         *types.OCIFilters
	}
	type fields struct {
		logger        *logrus.Entry
		instanceSvcFn func(t *testing.T, args *args) cloud.OCIInstanceService
		networkSvcFn  func(t *testing.T, args *args) cloud.OCINetworkService
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    string
		wantErr error
	}{
		{
			name: "assign reserved public IP",
			fields: fields{
				logger: logrus.NewEntry(logrus.New()),
				instanceSvcFn: func(t *testing.T, args *args) cloud.OCIInstanceService {
					mockSvc := cmocks.NewOCIInstanceService(t)
					mockSvc.EXPECT().ListVnicAttachments(mock.Anything, args.compartmentOCID, args.instanceOCID).Return([]core.VnicAttachment{
						{VnicId: common.String("test-vnic-id")},
					}, nil).Once()
					return mockSvc
				},
				networkSvcFn: func(t *testing.T, args *args) cloud.OCINetworkService {
					mockSvc := cmocks.NewOCINetworkService(t)
					mockSvc.EXPECT().GetPrimaryVnic(mock.Anything, mock.Anything).Return(&core.Vnic{
						Id: common.String("test-vnic-id"),
					}, nil).Once()
					mockSvc.EXPECT().GetPrimaryPrivateIPOfVnic(mock.Anything, mock.Anything).Return(&core.PrivateIp{
						Id: common.String("test-private-ip-id"),
					}, nil).Once()
					mockSvc.EXPECT().ListPublicIps(mock.Anything, mock.Anything, mock.Anything).Return([]core.PublicIp{
						{Id: common.String("test-public-ip"), IpAddress: common.String("1.2.3.4"), LifecycleState: core.PublicIpLifecycleStateAvailable},
					}, nil).Once()
					mockSvc.EXPECT().GetPublicIP(mock.Anything, mock.Anything).Return(&core.PublicIp{
						Id: common.String("test-public-ip"), IpAddress: common.String("1.2.3.4"), LifecycleState: core.PublicIpLifecycleStateAvailable,
					}, nil).Once()
					mockSvc.EXPECT().UpdatePublicIP(mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()
					return mockSvc
				},
			},
			args: args{
				compartmentOCID: "test-compartment-id",
				instanceOCID:    "test-instance-id",
			},
			want: "1.2.3.4",
		},
		{
			name: "failed to get primary VNIC of instance",
			fields: fields{
				logger: logrus.NewEntry(logrus.New()),
				instanceSvcFn: func(t *testing.T, args *args) cloud.OCIInstanceService {
					mockSvc := cmocks.NewOCIInstanceService(t)
					mockSvc.EXPECT().ListVnicAttachments(mock.Anything, args.compartmentOCID, args.instanceOCID).Return([]core.VnicAttachment{}, errors.New("error")).Once()
					return mockSvc
				},
				networkSvcFn: func(t *testing.T, args *args) cloud.OCINetworkService {
					return cmocks.NewOCINetworkService(t)
				},
			},
			args: args{
				compartmentOCID: "test-compartment-id",
				instanceOCID:    "test-instance-id",
			},
			wantErr: errors.New("failed to list VNIC attachments: error"),
		},
		{
			name: "failed to check public IP already assigned",
			fields: fields{
				logger: logrus.NewEntry(logrus.New()),
				instanceSvcFn: func(t *testing.T, args *args) cloud.OCIInstanceService {
					mockSvc := cmocks.NewOCIInstanceService(t)
					mockSvc.EXPECT().ListVnicAttachments(mock.Anything, args.compartmentOCID, args.instanceOCID).Return([]core.VnicAttachment{
						{VnicId: common.String("test-vnic-id")},
					}, nil).Once()
					return mockSvc
				},
				networkSvcFn: func(t *testing.T, args *args) cloud.OCINetworkService {
					mockSvc := cmocks.NewOCINetworkService(t)
					mockSvc.EXPECT().GetPrimaryVnic(mock.Anything, mock.Anything).Return(&core.Vnic{
						Id:       common.String("test-vnic-id"),
						PublicIp: common.String("1.2.3.4"),
					}, nil).Once()
					mockSvc.EXPECT().ListPublicIps(mock.Anything, mock.Anything, mock.Anything).Return(nil, errors.New("error")).Once()
					return mockSvc
				},
			},
			args: args{
				compartmentOCID: "test-compartment-id",
				instanceOCID:    "test-instance-id",
			},
			wantErr: errors.New("failed to check if public ip is already assigned or not: failed to list reserved public IPs assigned to private IP: failed to list public IPs: error"),
		},
		{
			name: "public IP already assigned from reserved list",
			fields: fields{
				logger: logrus.NewEntry(logrus.New()),
				instanceSvcFn: func(t *testing.T, args *args) cloud.OCIInstanceService {
					mockSvc := cmocks.NewOCIInstanceService(t)
					mockSvc.EXPECT().ListVnicAttachments(mock.Anything, args.compartmentOCID, args.instanceOCID).Return([]core.VnicAttachment{
						{VnicId: common.String("test-vnic-id")},
					}, nil).Once()
					return mockSvc
				},
				networkSvcFn: func(t *testing.T, args *args) cloud.OCINetworkService {
					mockSvc := cmocks.NewOCINetworkService(t)
					mockSvc.EXPECT().GetPrimaryVnic(mock.Anything, mock.Anything).Return(&core.Vnic{
						Id:       common.String("test-vnic-id"),
						PublicIp: common.String("1.2.3.4"),
					}, nil).Once()
					mockSvc.EXPECT().ListPublicIps(mock.Anything, mock.Anything, mock.Anything).Return([]core.PublicIp{
						{Id: common.String("test-public-ip"), IpAddress: common.String("1.2.3.4"), LifecycleState: core.PublicIpLifecycleStateAssigned},
					}, nil).Once()
					return mockSvc
				},
			},
			args: args{
				compartmentOCID: "test-compartment-id",
				instanceOCID:    "test-instance-id",
			},
			want:    "1.2.3.4",
			wantErr: ErrStaticIPAlreadyAssigned,
		},
		{
			name: "failed to get primary private IP of VNIC",
			fields: fields{
				logger: logrus.NewEntry(logrus.New()),
				instanceSvcFn: func(t *testing.T, args *args) cloud.OCIInstanceService {
					mockSvc := cmocks.NewOCIInstanceService(t)
					mockSvc.EXPECT().ListVnicAttachments(mock.Anything, args.compartmentOCID, args.instanceOCID).Return([]core.VnicAttachment{
						{VnicId: common.String("test-vnic-id")},
					}, nil).Once()
					return mockSvc
				},
				networkSvcFn: func(t *testing.T, args *args) cloud.OCINetworkService {
					mockSvc := cmocks.NewOCINetworkService(t)
					mockSvc.EXPECT().GetPrimaryVnic(mock.Anything, mock.Anything).Return(&core.Vnic{
						Id: common.String("test-vnic-id"),
					}, nil).Once()
					mockSvc.EXPECT().GetPrimaryPrivateIPOfVnic(mock.Anything, mock.Anything).Return(nil, errors.New("error")).Once()
					return mockSvc
				},
			},
			args: args{
				compartmentOCID: "test-compartment-id",
				instanceOCID:    "test-instance-id",
			},
			wantErr: errors.New("failed to get primary VNIC private IP: error"),
		},
		{
			name: "failed to fetch reserved public IP list",
			fields: fields{
				logger: logrus.NewEntry(logrus.New()),
				instanceSvcFn: func(t *testing.T, args *args) cloud.OCIInstanceService {
					mockSvc := cmocks.NewOCIInstanceService(t)
					mockSvc.EXPECT().ListVnicAttachments(mock.Anything, args.compartmentOCID, args.instanceOCID).Return([]core.VnicAttachment{
						{VnicId: common.String("test-vnic-id")},
					}, nil).Once()
					return mockSvc
				},
				networkSvcFn: func(t *testing.T, args *args) cloud.OCINetworkService {
					mockSvc := cmocks.NewOCINetworkService(t)
					mockSvc.EXPECT().GetPrimaryVnic(mock.Anything, mock.Anything).Return(&core.Vnic{
						Id: common.String("test-vnic-id"),
					}, nil).Once()
					mockSvc.EXPECT().GetPrimaryPrivateIPOfVnic(mock.Anything, mock.Anything).Return(&core.PrivateIp{
						Id: common.String("test-private-ip-id"),
					}, nil).Once()
					mockSvc.EXPECT().ListPublicIps(mock.Anything, mock.Anything, mock.Anything).Return(nil, errors.New("error")).Once()
					return mockSvc
				},
			},
			args: args{
				compartmentOCID: "test-compartment-id",
				instanceOCID:    "test-instance-id",
			},
			wantErr: errors.New("failed to get list of reserved public IPs: failed to list public IPs: error"),
		},
		{
			name: "no reserved public IP available to assign",
			fields: fields{
				logger: logrus.NewEntry(logrus.New()),
				instanceSvcFn: func(t *testing.T, args *args) cloud.OCIInstanceService {
					mockSvc := cmocks.NewOCIInstanceService(t)
					mockSvc.EXPECT().ListVnicAttachments(mock.Anything, args.compartmentOCID, args.instanceOCID).Return([]core.VnicAttachment{
						{VnicId: common.String("test-vnic-id")},
					}, nil).Once()
					return mockSvc
				},
				networkSvcFn: func(t *testing.T, args *args) cloud.OCINetworkService {
					mockSvc := cmocks.NewOCINetworkService(t)
					mockSvc.EXPECT().GetPrimaryVnic(mock.Anything, mock.Anything).Return(&core.Vnic{
						Id: common.String("test-vnic-id"),
					}, nil).Once()
					mockSvc.EXPECT().GetPrimaryPrivateIPOfVnic(mock.Anything, mock.Anything).Return(&core.PrivateIp{
						Id: common.String("test-private-ip-id"),
					}, nil).Once()
					mockSvc.EXPECT().ListPublicIps(mock.Anything, mock.Anything, mock.Anything).Return([]core.PublicIp{}, nil).Once()
					return mockSvc
				},
			},
			args: args{
				compartmentOCID: "test-compartment-id",
				instanceOCID:    "test-instance-id",
			},
			wantErr: errors.New("no reserved public IPs available"),
		},
		{
			name: "failed to assign public IP to private IP",
			fields: fields{
				logger: logrus.NewEntry(logrus.New()),
				instanceSvcFn: func(t *testing.T, args *args) cloud.OCIInstanceService {
					mockSvc := cmocks.NewOCIInstanceService(t)
					mockSvc.EXPECT().ListVnicAttachments(mock.Anything, args.compartmentOCID, args.instanceOCID).Return([]core.VnicAttachment{
						{VnicId: common.String("test-vnic-id")},
					}, nil).Once()
					return mockSvc
				},
				networkSvcFn: func(t *testing.T, args *args) cloud.OCINetworkService {
					mockSvc := cmocks.NewOCINetworkService(t)
					mockSvc.EXPECT().GetPrimaryVnic(mock.Anything, mock.Anything).Return(&core.Vnic{
						Id: common.String("test-vnic-id"),
					}, nil).Once()
					mockSvc.EXPECT().GetPrimaryPrivateIPOfVnic(mock.Anything, mock.Anything).Return(&core.PrivateIp{
						Id: common.String("test-private-ip-id"),
					}, nil).Once()
					mockSvc.EXPECT().ListPublicIps(mock.Anything, mock.Anything, mock.Anything).Return([]core.PublicIp{
						{Id: common.String("test-public-ip"), IpAddress: common.String("1.2.3.4"), LifecycleState: core.PublicIpLifecycleStateAvailable},
					}, nil).Once()
					mockSvc.EXPECT().GetPublicIP(mock.Anything, mock.Anything).Return(&core.PublicIp{
						Id: common.String("test-public-ip"), IpAddress: common.String("1.2.3.4"), LifecycleState: core.PublicIpLifecycleStateAvailable,
					}, nil).Once()
					mockSvc.EXPECT().UpdatePublicIP(mock.Anything, mock.Anything, mock.Anything).Return(errors.New("error")).Once()
					return mockSvc
				},
			},
			args: args{
				compartmentOCID: "test-compartment-id",
				instanceOCID:    "test-instance-id",
			},
			wantErr: errors.New("failed to assign any IP"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &ociAssigner{
				logger:          tt.fields.logger,
				filters:         tt.args.filters,
				compartmentOCID: tt.args.compartmentOCID,
				instanceSvc:     tt.fields.instanceSvcFn(t, &tt.args),
				networkSvc:      tt.fields.networkSvcFn(t, &tt.args),
			}
			got, err := a.Assign(context.TODO(), tt.args.instanceOCID, "", nil, "")
			if !matchErr(err, tt.wantErr) {
				t.Errorf("OCI Assign() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("OCI Assign() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_ociAssigner_Unassign(t *testing.T) {
	type args struct {
		compartmentOCID string
		instanceOCID    string
	}
	type fields struct {
		logger        *logrus.Entry
		instanceSvcFn func(t *testing.T, args *args) cloud.OCIInstanceService
		networkSvcFn  func(t *testing.T, args *args) cloud.OCINetworkService
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr error
	}{
		{
			name: "unassign public IP",
			fields: fields{
				logger: logrus.NewEntry(logrus.New()),
				instanceSvcFn: func(t *testing.T, args *args) cloud.OCIInstanceService {
					mockSvc := cmocks.NewOCIInstanceService(t)
					mockSvc.EXPECT().ListVnicAttachments(mock.Anything, args.compartmentOCID, args.instanceOCID).Return([]core.VnicAttachment{
						{VnicId: common.String("test-vnic-id")},
					}, nil).Once()
					return mockSvc
				},
				networkSvcFn: func(t *testing.T, args *args) cloud.OCINetworkService {
					mockSvc := cmocks.NewOCINetworkService(t)
					mockSvc.EXPECT().GetPrimaryVnic(mock.Anything, mock.Anything).Return(&core.Vnic{
						Id:       common.String("test-vnic-id"),
						PublicIp: common.String("1.2.3.4"),
					}, nil).Once()
					mockSvc.EXPECT().ListPublicIps(mock.Anything, mock.Anything, mock.Anything).Return([]core.PublicIp{
						{Id: common.String("test-public-ip"), IpAddress: common.String("1.2.3.4"), LifecycleState: core.PublicIpLifecycleStateAssigned},
					}, nil).Once()
					mockSvc.EXPECT().UpdatePublicIP(mock.Anything, "test-public-ip", "").Return(nil).Once()
					return mockSvc
				},
			},
			args: args{
				compartmentOCID: "test-compartment-id",
				instanceOCID:    "test-instance-id",
			},
		},
		{
			name: "public ip not assigned from reserved list",
			fields: fields{
				logger: logrus.NewEntry(logrus.New()),
				instanceSvcFn: func(t *testing.T, args *args) cloud.OCIInstanceService {
					mockSvc := cmocks.NewOCIInstanceService(t)
					mockSvc.EXPECT().ListVnicAttachments(mock.Anything, args.compartmentOCID, args.instanceOCID).Return([]core.VnicAttachment{
						{VnicId: common.String("test-vnic-id")},
					}, nil).Once()
					return mockSvc
				},
				networkSvcFn: func(t *testing.T, args *args) cloud.OCINetworkService {
					mockSvc := cmocks.NewOCINetworkService(t)
					mockSvc.EXPECT().GetPrimaryVnic(mock.Anything, mock.Anything).Return(&core.Vnic{
						Id:       common.String("test-vnic-id"),
						PublicIp: common.String("1.2.3.4"),
					}, nil).Once()
					mockSvc.EXPECT().ListPublicIps(mock.Anything, mock.Anything, mock.Anything).Return([]core.PublicIp{}, nil).Once()
					return mockSvc
				},
			},
			args: args{
				compartmentOCID: "test-compartment-id",
				instanceOCID:    "test-instance-id",
			},
			wantErr: errors.New("public IP not assigned from reserved list"),
		},
		{
			name: "failed to get primary VNIC of instance",
			fields: fields{
				logger: logrus.NewEntry(logrus.New()),
				instanceSvcFn: func(t *testing.T, args *args) cloud.OCIInstanceService {
					mockSvc := cmocks.NewOCIInstanceService(t)
					mockSvc.EXPECT().ListVnicAttachments(mock.Anything, args.compartmentOCID, args.instanceOCID).Return([]core.VnicAttachment{}, errors.New("error")).Once()
					return mockSvc
				},
				networkSvcFn: func(t *testing.T, args *args) cloud.OCINetworkService {
					return cmocks.NewOCINetworkService(t)
				},
			},
			args: args{
				compartmentOCID: "test-compartment-id",
				instanceOCID:    "test-instance-id",
			},
			wantErr: errors.New("failed to list VNIC attachments: error"),
		},
		{
			name: "no public IP assigned",
			fields: fields{
				logger: logrus.NewEntry(logrus.New()),
				instanceSvcFn: func(t *testing.T, args *args) cloud.OCIInstanceService {
					mockSvc := cmocks.NewOCIInstanceService(t)
					mockSvc.EXPECT().ListVnicAttachments(mock.Anything, args.compartmentOCID, args.instanceOCID).Return([]core.VnicAttachment{
						{VnicId: common.String("test-vnic-id")},
					}, nil).Once()
					return mockSvc
				},
				networkSvcFn: func(t *testing.T, args *args) cloud.OCINetworkService {
					mockSvc := cmocks.NewOCINetworkService(t)
					mockSvc.EXPECT().GetPrimaryVnic(mock.Anything, mock.Anything).Return(&core.Vnic{
						Id: common.String("test-vnic-id"),
					}, nil).Once()
					return mockSvc
				},
			},
			args: args{
				compartmentOCID: "test-compartment-id",
				instanceOCID:    "test-instance-id",
			},
			wantErr: ErrNoPublicIPAssigned,
		},
		{
			name: "failed to fetch assigned public IP",
			fields: fields{
				logger: logrus.NewEntry(logrus.New()),
				instanceSvcFn: func(t *testing.T, args *args) cloud.OCIInstanceService {
					mockSvc := cmocks.NewOCIInstanceService(t)
					mockSvc.EXPECT().ListVnicAttachments(mock.Anything, args.compartmentOCID, args.instanceOCID).Return([]core.VnicAttachment{
						{VnicId: common.String("test-vnic-id")},
					}, nil).Once()
					return mockSvc
				},
				networkSvcFn: func(t *testing.T, args *args) cloud.OCINetworkService {
					mockSvc := cmocks.NewOCINetworkService(t)
					mockSvc.EXPECT().GetPrimaryVnic(mock.Anything, mock.Anything).Return(&core.Vnic{
						Id:       common.String("test-vnic-id"),
						PublicIp: common.String("1.2.3.4"),
					}, nil).Once()
					mockSvc.EXPECT().ListPublicIps(mock.Anything, mock.Anything, mock.Anything).Return([]core.PublicIp{}, errors.New("error")).Once()
					return mockSvc
				},
			},
			args: args{
				compartmentOCID: "test-compartment-id",
				instanceOCID:    "test-instance-id",
			},
			wantErr: errors.New("failed to get list of reserved public IPs: failed to list public IPs: error"),
		},
		{
			name: "failed to update public IP",
			fields: fields{
				logger: logrus.NewEntry(logrus.New()),
				instanceSvcFn: func(t *testing.T, args *args) cloud.OCIInstanceService {
					mockSvc := cmocks.NewOCIInstanceService(t)
					mockSvc.EXPECT().ListVnicAttachments(mock.Anything, args.compartmentOCID, args.instanceOCID).Return([]core.VnicAttachment{
						{VnicId: common.String("test-vnic-id")},
					}, nil).Once()
					return mockSvc
				},
				networkSvcFn: func(t *testing.T, args *args) cloud.OCINetworkService {
					mockSvc := cmocks.NewOCINetworkService(t)
					mockSvc.EXPECT().GetPrimaryVnic(mock.Anything, mock.Anything).Return(&core.Vnic{
						Id:       common.String("test-vnic-id"),
						PublicIp: common.String("1.2.3.4"),
					}, nil).Once()
					mockSvc.EXPECT().ListPublicIps(mock.Anything, mock.Anything, mock.Anything).Return([]core.PublicIp{
						{Id: common.String("test-public-ip"), IpAddress: common.String("1.2.3.4"), LifecycleState: core.PublicIpLifecycleStateAssigned},
					}, nil).Once()
					mockSvc.EXPECT().UpdatePublicIP(mock.Anything, "test-public-ip", "").Return(errors.New("error")).Once()
					return mockSvc
				},
			},
			args: args{
				compartmentOCID: "test-compartment-id",
				instanceOCID:    "test-instance-id",
			},
			wantErr: errors.New("failed to unassign public IP assigned to private IP: error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &ociAssigner{
				logger:          tt.fields.logger,
				instanceSvc:     tt.fields.instanceSvcFn(t, &tt.args),
				networkSvc:      tt.fields.networkSvcFn(t, &tt.args),
				compartmentOCID: tt.args.compartmentOCID,
			}
			err := a.Unassign(context.TODO(), tt.args.instanceOCID, "")
			if !matchErr(err, tt.wantErr) {
				t.Errorf("Unassign() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_ociAssigner_getPrimaryVnicOfInstance(t *testing.T) {
	type args struct {
		compartmentOCID string
		instanceOCID    string
	}
	type fields struct {
		instanceSvcFn func(t *testing.T, args *args) cloud.OCIInstanceService
		networkSvcFn  func(t *testing.T, args *args) cloud.OCINetworkService
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *core.Vnic
		wantErr error
	}{
		{
			name: "get primary VNIC of instance",
			fields: fields{
				instanceSvcFn: func(t *testing.T, args *args) cloud.OCIInstanceService {
					mockSvc := cmocks.NewOCIInstanceService(t)
					mockSvc.EXPECT().ListVnicAttachments(mock.Anything, args.compartmentOCID, args.instanceOCID).Return([]core.VnicAttachment{
						{VnicId: common.String("test-vnic-id")},
					}, nil).Once()
					return mockSvc
				},
				networkSvcFn: func(t *testing.T, args *args) cloud.OCINetworkService {
					mockSvc := cmocks.NewOCINetworkService(t)
					mockSvc.EXPECT().GetPrimaryVnic(mock.Anything, mock.Anything).Return(&core.Vnic{
						Id: common.String("test-vnic-id"),
					}, nil).Once()
					return mockSvc
				},
			},
			args: args{
				compartmentOCID: "test-compartment-id",
				instanceOCID:    "test-instance-id",
			},
			want: &core.Vnic{
				Id: common.String("test-vnic-id"),
			},
		},
		{
			name: "failed to list VNIC attachments",
			fields: fields{
				instanceSvcFn: func(t *testing.T, args *args) cloud.OCIInstanceService {
					mockSvc := cmocks.NewOCIInstanceService(t)
					mockSvc.EXPECT().ListVnicAttachments(mock.Anything, args.compartmentOCID, args.instanceOCID).Return(nil, errors.New("error")).Once()
					return mockSvc
				},
				networkSvcFn: func(t *testing.T, args *args) cloud.OCINetworkService {
					return cmocks.NewOCINetworkService(t)
				},
			},
			args: args{
				compartmentOCID: "test-compartment-id",
				instanceOCID:    "test-instance-id",
			},
			wantErr: errors.New("failed to list VNIC attachments: error"),
		},
		{
			name: "failed to get primary VNIC of instance",
			fields: fields{
				instanceSvcFn: func(t *testing.T, args *args) cloud.OCIInstanceService {
					mockSvc := cmocks.NewOCIInstanceService(t)
					mockSvc.EXPECT().ListVnicAttachments(mock.Anything, args.compartmentOCID, args.instanceOCID).Return([]core.VnicAttachment{
						{VnicId: common.String("test-vnic-id")},
					}, nil).Once()
					return mockSvc
				},
				networkSvcFn: func(t *testing.T, args *args) cloud.OCINetworkService {
					mockSvc := cmocks.NewOCINetworkService(t)
					mockSvc.EXPECT().GetPrimaryVnic(mock.Anything, mock.Anything).Return(nil, errors.New("error")).Once()
					return mockSvc
				},
			},
			args: args{
				compartmentOCID: "test-compartment-id",
				instanceOCID:    "test-instance-id",
			},
			wantErr: errors.New("failed to get primary VNIC: error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &ociAssigner{
				instanceSvc:     tt.fields.instanceSvcFn(t, &tt.args),
				networkSvc:      tt.fields.networkSvcFn(t, &tt.args),
				compartmentOCID: tt.args.compartmentOCID,
			}
			got, err := a.getPrimaryVnicOfInstance(context.TODO(), tt.args.instanceOCID)
			if !matchErr(err, tt.wantErr) {
				t.Errorf("getPrimaryVnicOfInstance() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getPrimaryVnicOfInstance() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_ociAssigner_handlePublicIPAlreadyAssignedCase(t *testing.T) {
	type args struct {
		vnic            *core.Vnic
		filter          *types.OCIFilters
		compartmentOCID string
	}
	type fields struct {
		logger       *logrus.Entry
		networkSvcFn func(t *testing.T, args *args) cloud.OCINetworkService
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    bool
		wantErr error
	}{
		{
			name: "no vnic",
			fields: fields{
				logger: logrus.NewEntry(logrus.New()),
				networkSvcFn: func(t *testing.T, args *args) cloud.OCINetworkService {
					return cmocks.NewOCINetworkService(t)
				},
			},
			want: false,
		},
		{
			name: "no public IP assigned",
			fields: fields{
				logger: logrus.NewEntry(logrus.New()),
				networkSvcFn: func(t *testing.T, args *args) cloud.OCINetworkService {
					return cmocks.NewOCINetworkService(t)
				},
			},
			args: args{
				vnic: &core.Vnic{},
			},
			want: false,
		},
		{
			name: "public IP already assigned from reserved list",
			fields: fields{
				logger: logrus.NewEntry(logrus.New()),
				networkSvcFn: func(t *testing.T, args *args) cloud.OCINetworkService {
					mockSvc := cmocks.NewOCINetworkService(t)
					mockSvc.EXPECT().ListPublicIps(mock.Anything, &core.ListPublicIpsRequest{
						Scope:         core.ListPublicIpsScopeRegion,
						CompartmentId: common.String(args.compartmentOCID),
					}, args.filter).Return([]core.PublicIp{
						{IpAddress: common.String("1.2.3.4"), LifecycleState: core.PublicIpLifecycleStateAssigned},
					}, nil).Once()
					return mockSvc
				},
			},
			args: args{
				vnic: &core.Vnic{
					PublicIp: common.String("1.2.3.4"),
				},
				compartmentOCID: "test-compartment-id",
			},
			want: true,
		},
		{
			name: "failed to check public IP already assigned from reserved list",
			fields: fields{
				logger: logrus.NewEntry(logrus.New()),
				networkSvcFn: func(t *testing.T, args *args) cloud.OCINetworkService {
					mockSvc := cmocks.NewOCINetworkService(t)
					mockSvc.EXPECT().ListPublicIps(mock.Anything, &core.ListPublicIpsRequest{
						Scope:         core.ListPublicIpsScopeRegion,
						CompartmentId: common.String(args.compartmentOCID),
					}, args.filter).Return([]core.PublicIp{}, errors.New("error")).Once()
					return mockSvc
				},
			},
			args: args{
				vnic: &core.Vnic{
					PublicIp: common.String("1.2.3.4"),
				},
				compartmentOCID: "test-compartment-id",
			},
			want:    false,
			wantErr: errors.New("failed to list reserved public IPs assigned to private IP: failed to list public IPs: error"),
		},
		{
			name: "public IP assigned but not from reserved list",
			fields: fields{
				logger: logrus.NewEntry(logrus.New()),
				networkSvcFn: func(t *testing.T, args *args) cloud.OCINetworkService {
					mockSvc := cmocks.NewOCINetworkService(t)
					mockSvc.EXPECT().ListPublicIps(mock.Anything, mock.Anything, mock.Anything).Return([]core.PublicIp{}, nil).Once()
					mockSvc.EXPECT().ListPublicIps(mock.Anything, &core.ListPublicIpsRequest{
						Scope:         core.ListPublicIpsScopeRegion,
						CompartmentId: common.String(args.compartmentOCID),
					}, args.filter).Return([]core.PublicIp{
						{Id: common.String("test-public-ip"), IpAddress: common.String("1.2.3.4"), LifecycleState: core.PublicIpLifecycleStateAssigned},
					}, nil).Once()
					mockSvc.EXPECT().UpdatePublicIP(mock.Anything, "test-public-ip", "").Return(nil).Once()
					return mockSvc
				},
			},
			args: args{
				vnic: &core.Vnic{
					PublicIp: common.String("1.2.3.4"),
				},
				compartmentOCID: "test-compartment-id",
			},
			want: false,
		},
		{
			name: "failed to list public IPs assigned to private IP",
			fields: fields{
				logger: logrus.NewEntry(logrus.New()),
				networkSvcFn: func(t *testing.T, args *args) cloud.OCINetworkService {
					mockSvc := cmocks.NewOCINetworkService(t)
					mockSvc.EXPECT().ListPublicIps(mock.Anything, mock.Anything, mock.Anything).Return([]core.PublicIp{}, nil).Once()
					mockSvc.EXPECT().ListPublicIps(mock.Anything, &core.ListPublicIpsRequest{
						Scope:         core.ListPublicIpsScopeRegion,
						CompartmentId: common.String(args.compartmentOCID),
					}, args.filter).Return([]core.PublicIp{}, errors.New("error")).Once()
					return mockSvc
				},
			},
			args: args{
				vnic: &core.Vnic{
					PublicIp: common.String("1.2.3.4"),
				},
				compartmentOCID: "test-compartment-id",
			},
			want:    false,
			wantErr: errors.New("failed to list public IPs assigned to private IP: failed to list public IPs: error"),
		},
		{
			name: "failed to update public ip",
			fields: fields{
				logger: logrus.NewEntry(logrus.New()),
				networkSvcFn: func(t *testing.T, args *args) cloud.OCINetworkService {
					mockSvc := cmocks.NewOCINetworkService(t)
					mockSvc.EXPECT().ListPublicIps(mock.Anything, mock.Anything, mock.Anything).Return([]core.PublicIp{}, nil).Once()
					mockSvc.EXPECT().ListPublicIps(mock.Anything, &core.ListPublicIpsRequest{
						Scope:         core.ListPublicIpsScopeRegion,
						CompartmentId: common.String(args.compartmentOCID),
					}, args.filter).Return([]core.PublicIp{
						{Id: common.String("test-public-ip"), IpAddress: common.String("1.2.3.4"), LifecycleState: core.PublicIpLifecycleStateAssigned},
					}, nil).Once()
					mockSvc.EXPECT().UpdatePublicIP(mock.Anything, "test-public-ip", "").Return(errors.New("error")).Once()
					return mockSvc
				},
			},
			args: args{
				vnic: &core.Vnic{
					PublicIp: common.String("1.2.3.4"),
				},
				compartmentOCID: "test-compartment-id",
			},
			want:    false,
			wantErr: errors.New("failed to unassign public IP assigned to private IP: error"),
		},
		{
			name: "ephemeral public IP assigned",
			fields: fields{
				logger: logrus.NewEntry(logrus.New()),
				networkSvcFn: func(t *testing.T, args *args) cloud.OCINetworkService {
					mockSvc := cmocks.NewOCINetworkService(t)
					mockSvc.EXPECT().ListPublicIps(mock.Anything, mock.Anything, mock.Anything).Return([]core.PublicIp{}, nil).Times(2)
					mockSvc.EXPECT().ListPublicIps(mock.Anything, &core.ListPublicIpsRequest{
						Scope:              core.ListPublicIpsScopeAvailabilityDomain,
						CompartmentId:      common.String(args.compartmentOCID),
						AvailabilityDomain: common.String("test-availability-domain"),
					}, args.filter).Return([]core.PublicIp{
						{Id: common.String("test-public-ip"), IpAddress: common.String("1.2.3.4"), LifecycleState: core.PublicIpLifecycleStateAssigned},
					}, nil).Once()
					mockSvc.EXPECT().DeletePublicIP(mock.Anything, "test-public-ip").Return(nil).Once()
					return mockSvc
				},
			},
			args: args{
				vnic: &core.Vnic{
					PublicIp:           common.String("1.2.3.4"),
					AvailabilityDomain: common.String("test-availability-domain"),
				},
				compartmentOCID: "test-compartment-id",
			},
			want: false,
		},
		{
			name: "failed to list ephemeral public IPs",
			fields: fields{
				logger: logrus.NewEntry(logrus.New()),
				networkSvcFn: func(t *testing.T, args *args) cloud.OCINetworkService {
					mockSvc := cmocks.NewOCINetworkService(t)
					mockSvc.EXPECT().ListPublicIps(mock.Anything, mock.Anything, mock.Anything).Return([]core.PublicIp{}, nil).Times(2)
					mockSvc.EXPECT().ListPublicIps(mock.Anything, &core.ListPublicIpsRequest{
						Scope:              core.ListPublicIpsScopeAvailabilityDomain,
						CompartmentId:      common.String(args.compartmentOCID),
						AvailabilityDomain: common.String("test-availability-domain"),
					}, args.filter).Return([]core.PublicIp{}, errors.New("error")).Once()
					return mockSvc
				},
			},
			args: args{
				vnic: &core.Vnic{
					PublicIp:           common.String("1.2.3.4"),
					AvailabilityDomain: common.String("test-availability-domain"),
				},
				compartmentOCID: "test-compartment-id",
			},
			want:    false,
			wantErr: errors.New("failed to list ephemeral public IPs assigned to private IP: failed to list ephemeral public IPs: error"),
		},
		{
			name: "failed to delete public IP",
			fields: fields{
				logger: logrus.NewEntry(logrus.New()),
				networkSvcFn: func(t *testing.T, args *args) cloud.OCINetworkService {
					mockSvc := cmocks.NewOCINetworkService(t)
					mockSvc.EXPECT().ListPublicIps(mock.Anything, mock.Anything, mock.Anything).Return([]core.PublicIp{}, nil).Times(2)
					mockSvc.EXPECT().ListPublicIps(mock.Anything, &core.ListPublicIpsRequest{
						Scope:              core.ListPublicIpsScopeAvailabilityDomain,
						CompartmentId:      common.String(args.compartmentOCID),
						AvailabilityDomain: common.String("test-availability-domain"),
					}, args.filter).Return([]core.PublicIp{
						{Id: common.String("test-public-ip"), IpAddress: common.String("1.2.3.4"), LifecycleState: core.PublicIpLifecycleStateAssigned},
					}, nil).Once()
					mockSvc.EXPECT().DeletePublicIP(mock.Anything, "test-public-ip").Return(errors.New("error")).Once()
					return mockSvc
				},
			},
			args: args{
				vnic: &core.Vnic{
					PublicIp:           common.String("1.2.3.4"),
					AvailabilityDomain: common.String("test-availability-domain"),
				},
				compartmentOCID: "test-compartment-id",
			},
			want:    false,
			wantErr: errors.New("failed to delete ephemeral public IP assigned to private IP: error"),
		},
		{
			name: "unhandled case",
			fields: fields{
				logger: logrus.NewEntry(logrus.New()),
				networkSvcFn: func(t *testing.T, args *args) cloud.OCINetworkService {
					mockSvc := cmocks.NewOCINetworkService(t)
					mockSvc.EXPECT().ListPublicIps(mock.Anything, mock.Anything, mock.Anything).Return([]core.PublicIp{}, nil).Times(2)
					return mockSvc
				},
			},
			args: args{
				vnic: &core.Vnic{
					PublicIp: common.String("1.2.3.4"),
				},
				compartmentOCID: "test-compartment-id",
				filter:          &types.OCIFilters{},
			},
			want:    false,
			wantErr: errors.New("availability domain not found"),
		},
		{
			name: "unhandled case",
			fields: fields{
				logger: logrus.NewEntry(logrus.New()),
				networkSvcFn: func(t *testing.T, args *args) cloud.OCINetworkService {
					mockSvc := cmocks.NewOCINetworkService(t)
					mockSvc.EXPECT().ListPublicIps(mock.Anything, mock.Anything, mock.Anything).Return([]core.PublicIp{}, nil).Times(3)
					return mockSvc
				},
			},
			args: args{
				vnic: &core.Vnic{
					PublicIp:           common.String("1.2.3.4"),
					AvailabilityDomain: common.String("test-availability-domain"),
				},
				compartmentOCID: "test-compartment-id",
			},
			want:    false,
			wantErr: errors.New("unhandled case: public IP is assigned to the instance but not from the reserved IP list"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &ociAssigner{
				logger:          tt.fields.logger,
				networkSvc:      tt.fields.networkSvcFn(t, &tt.args),
				compartmentOCID: tt.args.compartmentOCID,
				filters:         tt.args.filter,
			}
			got, err := a.handlePublicIPAlreadyAssignedCase(context.TODO(), tt.args.vnic)
			if !matchErr(err, tt.wantErr) {
				t.Errorf("handlePublicIPAlreadyAssignedCase() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("handlePublicIPAlreadyAssignedCase() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_ociAssigner_fetchPublicIps(t *testing.T) {
	type args struct {
		useFilter       bool
		inUSe           bool
		compartmentOCID string
		filters         *types.OCIFilters
	}
	type fields struct {
		networkSvcFn func(t *testing.T, args *args) cloud.OCINetworkService
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    []core.PublicIp
		wantErr error
	}{
		{
			name: "fetch public IPs",
			fields: fields{
				networkSvcFn: func(t *testing.T, args *args) cloud.OCINetworkService {
					mockSvc := cmocks.NewOCINetworkService(t)
					mockSvc.EXPECT().ListPublicIps(mock.Anything, &core.ListPublicIpsRequest{
						CompartmentId: common.String(args.compartmentOCID),
						Scope:         core.ListPublicIpsScopeRegion,
					}, args.filters).Return([]core.PublicIp{
						{Id: common.String("test-public-ip-id"), IpAddress: common.String("test-ip-address"), LifecycleState: core.PublicIpLifecycleStateAvailable},
					}, nil).Once()
					return mockSvc
				},
			},
			args: args{
				useFilter:       false,
				inUSe:           false,
				compartmentOCID: "test-compartment-id",
			},
			want: []core.PublicIp{
				{Id: common.String("test-public-ip-id"), IpAddress: common.String("test-ip-address"), LifecycleState: core.PublicIpLifecycleStateAvailable},
			},
		},
		{
			name: "fetch public IPs with filter",
			fields: fields{
				networkSvcFn: func(t *testing.T, args *args) cloud.OCINetworkService {
					mockSvc := cmocks.NewOCINetworkService(t)
					mockSvc.EXPECT().ListPublicIps(mock.Anything, &core.ListPublicIpsRequest{
						CompartmentId: common.String(args.compartmentOCID),
						Scope:         core.ListPublicIpsScopeRegion,
					}, args.filters).Return([]core.PublicIp{
						{Id: common.String("test-public-ip-id"), IpAddress: common.String("test-ip-address"), LifecycleState: core.PublicIpLifecycleStateAvailable},
					}, nil).Once()
					return mockSvc
				},
			},
			args: args{
				useFilter:       true,
				inUSe:           false,
				compartmentOCID: "test-compartment-id",
				filters: &types.OCIFilters{
					FreeformTags: map[string]string{"kubeip": "reserved"},
				},
			},
			want: []core.PublicIp{
				{Id: common.String("test-public-ip-id"), IpAddress: common.String("test-ip-address"), LifecycleState: core.PublicIpLifecycleStateAvailable},
			},
		},
		{
			name: "fetch public IPs in use",
			fields: fields{
				networkSvcFn: func(t *testing.T, args *args) cloud.OCINetworkService {
					mockSvc := cmocks.NewOCINetworkService(t)
					mockSvc.EXPECT().ListPublicIps(mock.Anything, &core.ListPublicIpsRequest{
						CompartmentId: common.String(args.compartmentOCID),
						Scope:         core.ListPublicIpsScopeRegion,
					}, args.filters).Return([]core.PublicIp{
						{Id: common.String("test-public-ip-id"), IpAddress: common.String("test-ip-address"), LifecycleState: core.PublicIpLifecycleStateAssigned},
					}, nil).Once()
					return mockSvc
				},
			},
			args: args{
				useFilter:       false,
				inUSe:           true,
				compartmentOCID: "test-compartment-id",
			},
			want: []core.PublicIp{
				{Id: common.String("test-public-ip-id"), IpAddress: common.String("test-ip-address"), LifecycleState: core.PublicIpLifecycleStateAssigned},
			},
		},
		{
			name: "failed to fetch public IPs",
			fields: fields{
				networkSvcFn: func(t *testing.T, args *args) cloud.OCINetworkService {
					mockSvc := cmocks.NewOCINetworkService(t)
					mockSvc.EXPECT().ListPublicIps(mock.Anything, &core.ListPublicIpsRequest{
						CompartmentId: common.String(args.compartmentOCID),
						Scope:         core.ListPublicIpsScopeRegion,
					}, args.filters).Return([]core.PublicIp{}, errors.New("failed to list public IPs")).Once()
					return mockSvc
				},
			},
			args: args{
				useFilter:       false,
				inUSe:           false,
				compartmentOCID: "test-compartment-id",
			},
			wantErr: errors.New("failed to list public IPs: failed to list public IPs"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &ociAssigner{
				networkSvc:      tt.fields.networkSvcFn(t, &tt.args),
				compartmentOCID: tt.args.compartmentOCID,
				filters:         tt.args.filters,
			}
			got, err := a.fetchPublicIps(context.TODO(), tt.args.useFilter, tt.args.inUSe)
			if !matchErr(err, tt.wantErr) {
				t.Errorf("OCI fetchPublicIps() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("OCI fetchPublicIps() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_ociAssigner_fetchEphemeralPublicIps(t *testing.T) {
	type args struct {
		availabilityDomain string
		compartmentOCID    string
		filters            *types.OCIFilters
	}
	type fields struct {
		networkSvcFn func(t *testing.T, args *args) cloud.OCINetworkService
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    []core.PublicIp
		wantErr error
	}{
		{
			name: "fetch ephemeral public IPs",
			fields: fields{
				networkSvcFn: func(t *testing.T, args *args) cloud.OCINetworkService {
					mockSvc := cmocks.NewOCINetworkService(t)
					mockSvc.EXPECT().ListPublicIps(mock.Anything, &core.ListPublicIpsRequest{
						CompartmentId:      common.String(args.compartmentOCID),
						Scope:              core.ListPublicIpsScopeAvailabilityDomain,
						AvailabilityDomain: common.String(args.availabilityDomain),
					}, args.filters).Return([]core.PublicIp{
						{Id: common.String("test-public-ip-id"), IpAddress: common.String("test-ip-address")},
					}, nil).Once()
					return mockSvc
				},
			},
			args: args{
				availabilityDomain: "test-availability-domain",
				compartmentOCID:    "test-compartment-id",
			},
			want: []core.PublicIp{
				{Id: common.String("test-public-ip-id"), IpAddress: common.String("test-ip-address")},
			},
		},
		{
			name: "failed to fetch ephemeral public IPs",
			fields: fields{
				networkSvcFn: func(t *testing.T, args *args) cloud.OCINetworkService {
					mockSvc := cmocks.NewOCINetworkService(t)
					mockSvc.EXPECT().ListPublicIps(mock.Anything, &core.ListPublicIpsRequest{
						CompartmentId:      common.String(args.compartmentOCID),
						Scope:              core.ListPublicIpsScopeAvailabilityDomain,
						AvailabilityDomain: common.String(args.availabilityDomain),
					}, args.filters).Return([]core.PublicIp{}, errors.New("failed to list public IPs")).Once()
					return mockSvc
				},
			},
			args: args{
				availabilityDomain: "test-availability-domain",
				compartmentOCID:    "test-compartment-id",
			},
			wantErr: errors.New("failed to list ephemeral public IPs: failed to list public IPs"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &ociAssigner{
				networkSvc:      tt.fields.networkSvcFn(t, &tt.args),
				compartmentOCID: tt.args.compartmentOCID,
				filters:         tt.args.filters,
			}
			got, err := a.fetchEphemeralPublicIPs(context.TODO(), tt.args.availabilityDomain)
			if !matchErr(err, tt.wantErr) {
				t.Errorf("OCI fetchEphemeralPublicIPs() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("OCI fetchEphemeralPublicIPs() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_ociAssigner_tryAssignAddress(t *testing.T) {
	type args struct {
		privateIPOCID string
		publicIP      string
	}
	type fields struct {
		networkSvcFn func(t *testing.T, args *args) cloud.OCINetworkService
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr error
	}{
		{
			name: "assign public IP",
			fields: fields{
				networkSvcFn: func(t *testing.T, args *args) cloud.OCINetworkService {
					mockSvc := cmocks.NewOCINetworkService(t)
					mockSvc.EXPECT().GetPublicIP(mock.Anything, "test-public-ip-id").Return(&core.PublicIp{
						Id:             common.String("test-public-ip-id"),
						LifecycleState: core.PublicIpLifecycleStateAvailable,
					}, nil).Once()
					mockSvc.EXPECT().UpdatePublicIP(mock.Anything, "test-public-ip-id", "test-private-ip-id").Return(nil).Once()
					return mockSvc
				},
			},
			args: args{
				privateIPOCID: "test-private-ip-id",
				publicIP:      "test-public-ip-id",
			},
		},
		{
			name: "failed to get public ip details",
			fields: fields{
				networkSvcFn: func(t *testing.T, args *args) cloud.OCINetworkService {
					mockSvc := cmocks.NewOCINetworkService(t)
					mockSvc.EXPECT().GetPublicIP(mock.Anything, "test-public-ip-id").Return(nil, errors.New("invalid ip id")).Once()
					return mockSvc
				},
			},
			args: args{
				privateIPOCID: "test-private-ip-id",
				publicIP:      "test-public-ip-id",
			},
			wantErr: errors.New("failed to get public IP details: invalid ip id"),
		},
		{
			name: "public IP detail is nil",
			fields: fields{
				networkSvcFn: func(t *testing.T, args *args) cloud.OCINetworkService {
					mockSvc := cmocks.NewOCINetworkService(t)
					mockSvc.EXPECT().GetPublicIP(mock.Anything, "test-public-ip-id").Return(nil, nil).Once()
					return mockSvc
				},
			},
			args: args{
				privateIPOCID: "test-private-ip-id",
				publicIP:      "test-public-ip-id",
			},
			wantErr: errors.New("public IP not found"),
		},
		{
			name: "public IP is not available",
			fields: fields{
				networkSvcFn: func(t *testing.T, args *args) cloud.OCINetworkService {
					mockSvc := cmocks.NewOCINetworkService(t)
					mockSvc.EXPECT().GetPublicIP(mock.Anything, "test-public-ip-id").Return(&core.PublicIp{
						Id:             common.String("test-public-ip-id"),
						LifecycleState: core.PublicIpLifecycleStateAssigned,
					}, nil).Once()
					return mockSvc
				},
			},
			args: args{
				privateIPOCID: "test-private-ip-id",
				publicIP:      "test-public-ip-id",
			},
			wantErr: errors.New("public IP is not available"),
		},
		{
			name: "failed to update public IP",
			fields: fields{
				networkSvcFn: func(t *testing.T, args *args) cloud.OCINetworkService {
					mockSvc := cmocks.NewOCINetworkService(t)
					mockSvc.EXPECT().GetPublicIP(mock.Anything, "test-public-ip-id").Return(&core.PublicIp{
						Id:             common.String("test-public-ip-id"),
						LifecycleState: core.PublicIpLifecycleStateAvailable,
					}, nil).Once()
					mockSvc.EXPECT().UpdatePublicIP(mock.Anything, "test-public-ip-id", "test-private-ip-id").Return(errors.New("error while update")).Once()
					return mockSvc
				},
			},
			args: args{
				privateIPOCID: "test-private-ip-id",
				publicIP:      "test-public-ip-id",
			},
			wantErr: errors.New("failed to assign public IP: error while update"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &ociAssigner{
				networkSvc: tt.fields.networkSvcFn(t, &tt.args),
			}
			if err := a.tryAssignAddress(context.TODO(), tt.args.privateIPOCID, tt.args.publicIP); !matchErr(err, tt.wantErr) {
				t.Errorf("OCI tryAssignAddress() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_parseOCIFilters(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *config.Config
		want    *types.OCIFilters
		wantErr error
	}{
		{
			name:    "no config",
			wantErr: errors.New("config is nil"),
		},
		{
			name: "valid freeformTags filter",
			cfg: &config.Config{
				Filter: []string{"freeformTags.key1=value1"},
			},
			want: &types.OCIFilters{
				FreeformTags: map[string]string{"key1": "value1"},
			},
		},
		{
			name: "invalid freeformTags filter",
			cfg: &config.Config{
				Filter: []string{"freeformTags.key1value1"},
			},
			wantErr: errors.New("failed to parse freeform tag filter: invalid filter format for freeform tags, should be in format freeformTags.key=value, found: freeformTags.key1value1"),
		},
		{
			name: "invalid filter format",
			cfg: &config.Config{
				Filter: []string{"invalidFilter"},
			},
			wantErr: errors.New("invalid filter format for OCI, should be in format freeformTags.key=value or definedTags.Namespace.key=value, found: invalidFilter"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseOCIFilters(tt.cfg)
			if !matchErr(err, tt.wantErr) {
				t.Errorf("parseOCIFilters() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseOCIFilters() = %v, want %v", got, tt.want)
			}
		})
	}
}
