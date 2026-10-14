package main

import (
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/doitintl/kubeip/internal/address"
	"github.com/doitintl/kubeip/internal/config"
	"github.com/doitintl/kubeip/internal/node"
	"github.com/doitintl/kubeip/internal/types"
	mocks "github.com/doitintl/kubeip/mocks/address"
	nodeMocks "github.com/doitintl/kubeip/mocks/node"
	"github.com/pkg/errors"
	tmock "github.com/stretchr/testify/mock"
	"k8s.io/client-go/kubernetes/fake"
)

func Test_assignAddress(t *testing.T) {
	type args struct {
		c          context.Context
		assignerFn func(t *testing.T) address.Assigner
		node       *types.Node
		cfg        *config.Config
	}
	tests := []struct {
		name    string
		args    args
		address string
		wantErr bool
	}{
		{
			name:    "assign address successfully",
			address: "1.1.1.1",
			args: args{
				c: context.Background(),
				assignerFn: func(t *testing.T) address.Assigner {
					mock := mocks.NewAssigner(t)
					mock.EXPECT().Assign(tmock.Anything, "test-instance", "test-zone", []string{"test-filter"}, "test-order-by").Return("1.1.1.1", nil)
					return mock
				},
				node: &types.Node{
					Name:     "test-node",
					Instance: "test-instance",
					Region:   "test-region",
					Zone:     "test-zone",
				},
				cfg: &config.Config{
					Filter:        []string{"test-filter"},
					OrderBy:       "test-order-by",
					RetryAttempts: 3,
					RetryInterval: time.Millisecond,
					LeaseDuration: 1,
				},
			},
		},
		{
			name:    "assign address after a few retries",
			address: "1.1.1.1",
			args: args{
				c: context.Background(),
				assignerFn: func(t *testing.T) address.Assigner {
					mock := mocks.NewAssigner(t)
					mock.EXPECT().Assign(tmock.Anything, "test-instance", "test-zone", []string{"test-filter"}, "test-order-by").Return("", errors.New("first error")).Once()
					mock.EXPECT().Assign(tmock.Anything, "test-instance", "test-zone", []string{"test-filter"}, "test-order-by").Return("", errors.New("second error")).Once()
					mock.EXPECT().Assign(tmock.Anything, "test-instance", "test-zone", []string{"test-filter"}, "test-order-by").Return("1.1.1.1", nil).Once()
					return mock
				},
				node: &types.Node{
					Name:     "test-node",
					Instance: "test-instance",
					Region:   "test-region",
					Zone:     "test-zone",
				},
				cfg: &config.Config{
					Filter:        []string{"test-filter"},
					OrderBy:       "test-order-by",
					RetryAttempts: 3,
					RetryInterval: time.Millisecond,
					LeaseDuration: 1,
				},
			},
		},
		{
			name: "error after a few retries and reached maximum number of retries",
			args: args{
				c: context.Background(),
				assignerFn: func(t *testing.T) address.Assigner {
					mock := mocks.NewAssigner(t)
					mock.EXPECT().Assign(tmock.Anything, "test-instance", "test-zone", []string{"test-filter"}, "test-order-by").Return("", errors.New("error")).Times(4)
					return mock
				},
				node: &types.Node{
					Name:     "test-node",
					Instance: "test-instance",
					Region:   "test-region",
					Zone:     "test-zone",
				},
				cfg: &config.Config{
					Filter:        []string{"test-filter"},
					OrderBy:       "test-order-by",
					RetryAttempts: 3,
					RetryInterval: time.Millisecond,
					LeaseDuration: 1,
				},
			},
			wantErr: true,
		},
		{
			name: "context cancelled while assigning addresses",
			args: args{
				c: func() context.Context {
					ctx, cancel := context.WithCancel(context.Background())
					go func() {
						// Simulate a shutdown signal being received after a short delay
						time.Sleep(20 * time.Millisecond)
						cancel()
					}()
					return ctx
				}(),
				assignerFn: func(t *testing.T) address.Assigner {
					mock := mocks.NewAssigner(t)
					mock.EXPECT().Assign(tmock.Anything, "test-instance", "test-zone", []string{"test-filter"}, "test-order-by").Return("", errors.New("error")).Maybe()
					return mock
				},
				node: &types.Node{
					Name:     "test-node",
					Instance: "test-instance",
					Region:   "test-region",
					Zone:     "test-zone",
				},
				cfg: &config.Config{
					Filter:        []string{"test-filter"},
					OrderBy:       "test-order-by",
					RetryAttempts: 10,
					RetryInterval: 5 * time.Millisecond,
					LeaseDuration: 1,
				},
			},
			wantErr: true,
		},
		{
			name: "error after a few retries and context is done",
			args: args{
				c: func() context.Context {
					ctx, _ := context.WithTimeout(context.Background(), 10*time.Millisecond) //nolint:govet
					return ctx
				}(),
				assignerFn: func(t *testing.T) address.Assigner {
					mock := mocks.NewAssigner(t)
					mock.EXPECT().Assign(tmock.Anything, "test-instance", "test-zone", []string{"test-filter"}, "test-order-by").Return("", errors.New("error")).Maybe()
					return mock
				},
				node: &types.Node{
					Name:     "test-node",
					Instance: "test-instance",
					Region:   "test-region",
					Zone:     "test-zone",
				},
				cfg: &config.Config{
					Filter:        []string{"test-filter"},
					OrderBy:       "test-order-by",
					RetryAttempts: 3,
					RetryInterval: 15 * time.Millisecond,
					LeaseDuration: 1,
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := prepareLogger("debug", false)
			assigner := tt.args.assignerFn(t)
			client := fake.NewSimpleClientset()
			assignedAddress, err := assignAddress(tt.args.c, log, client, assigner, tt.args.node, tt.args.cfg)
			if err != nil != tt.wantErr {
				t.Errorf("assignAddress() error = %v, wantErr %v", err, tt.wantErr)
			} else if assignedAddress != tt.address {
				t.Fatalf("assignAddress() = %v, want %v", assignedAddress, tt.address)
			}
		})
	}
}

func Test_waitForAddressToBeReported(t *testing.T) {
	type args struct {
		c          context.Context
		explorerFn func(t *testing.T) node.Explorer
		node       *types.Node
		address    string
		cfg        *config.Config
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "address reported with no retries",
			args: args{
				c:       context.Background(),
				address: "1.1.1.1",
				explorerFn: func(t *testing.T) node.Explorer {
					mock := nodeMocks.NewExplorer(t)
					mock.EXPECT().GetNode(tmock.Anything, "test-node").Return(
						&types.Node{
							Name:        "test-node",
							Instance:    "test-instance",
							Region:      "test-region",
							Zone:        "test-zone",
							ExternalIPs: []net.IP{net.IPv4(1, 1, 1, 1)},
						},
						nil,
					)
					return mock
				},
				node: &types.Node{
					Name:     "test-node",
					Instance: "test-instance",
					Region:   "test-region",
					Zone:     "test-zone",
				},
				cfg: &config.Config{
					WaitForNodeAddress: true,
					Filter:             []string{"test-filter"},
					OrderBy:            "test-order-by",
					RetryAttempts:      3,
					RetryInterval:      time.Millisecond,
					LeaseDuration:      1,
				},
			},
		},
		{
			name: "address reported after a few retries",
			args: args{
				c:       context.Background(),
				address: "1.1.1.1",
				explorerFn: func(t *testing.T) node.Explorer {
					mock := nodeMocks.NewExplorer(t)
					mock.EXPECT().GetNode(tmock.Anything, "test-node").Return(&types.Node{
						Name:        "test-node",
						Instance:    "test-instance",
						Region:      "test-region",
						Zone:        "test-zone",
						ExternalIPs: []net.IP{net.IPv4(9, 9, 9, 9)},
					}, nil).Times(3)
					mock.EXPECT().GetNode(tmock.Anything, "test-node").Return(&types.Node{
						Name:        "test-node",
						Instance:    "test-instance",
						Region:      "test-region",
						Zone:        "test-zone",
						ExternalIPs: []net.IP{net.IPv4(1, 1, 1, 1)},
					}, nil).Once()
					return mock
				},
				node: &types.Node{
					Name:     "test-node",
					Instance: "test-instance",
					Region:   "test-region",
					Zone:     "test-zone",
				},
				cfg: &config.Config{
					WaitForNodeAddress: true,
					Filter:             []string{"test-filter"},
					OrderBy:            "test-order-by",
					RetryAttempts:      3,
					RetryInterval:      time.Millisecond,
					LeaseDuration:      1,
				},
			},
		},
		{
			name: "error after a few retries and reached maximum number of retries",
			args: args{
				c: context.Background(),
				explorerFn: func(t *testing.T) node.Explorer {
					mock := nodeMocks.NewExplorer(t)
					mock.EXPECT().GetNode(tmock.Anything, "test-node").Return(&types.Node{
						Name:        "test-node",
						Instance:    "test-instance",
						Region:      "test-region",
						Zone:        "test-zone",
						ExternalIPs: []net.IP{net.IPv4(9, 9, 9, 9)},
					}, nil).Times(4)
					mock.EXPECT().GetNode(tmock.Anything, "test-node").Return(&types.Node{
						Name:        "test-node",
						Instance:    "test-instance",
						Region:      "test-region",
						Zone:        "test-zone",
						ExternalIPs: []net.IP{net.IPv4(1, 1, 1, 1)},
					}, nil).Times(0)
					return mock
				},
				node: &types.Node{
					Name:     "test-node",
					Instance: "test-instance",
					Region:   "test-region",
					Zone:     "test-zone",
				},
				cfg: &config.Config{
					WaitForNodeAddress: true,
					Filter:             []string{"test-filter"},
					OrderBy:            "test-order-by",
					RetryAttempts:      3,
					RetryInterval:      time.Millisecond,
					LeaseDuration:      1,
				},
			},
			wantErr: true,
		},
		{
			name: "context cancelled while waiting for address to be reported",
			args: args{
				c: func() context.Context {
					ctx, cancel := context.WithCancel(context.Background())
					go func() {
						// Simulate a shutdown signal being received after a short delay
						time.Sleep(20 * time.Millisecond)
						cancel()
					}()
					return ctx
				}(),
				explorerFn: func(t *testing.T) node.Explorer {
					mock := nodeMocks.NewExplorer(t)
					mock.EXPECT().GetNode(tmock.Anything, "test-node").Return(nil, errors.New("error")).Maybe()
					return mock
				},
				node: &types.Node{
					Name:     "test-node",
					Instance: "test-instance",
					Region:   "test-region",
					Zone:     "test-zone",
				},
				cfg: &config.Config{
					WaitForNodeAddress: true,
					Filter:             []string{"test-filter"},
					OrderBy:            "test-order-by",
					RetryAttempts:      10,
					RetryInterval:      5 * time.Millisecond,
					LeaseDuration:      1,
				},
			},
			wantErr: true,
		},
		{
			name: "error after a few retries and context is done",
			args: args{
				c: func() context.Context {
					ctx, _ := context.WithTimeout(context.Background(), 10*time.Millisecond) //nolint:govet
					return ctx
				}(),
				explorerFn: func(t *testing.T) node.Explorer {
					mock := nodeMocks.NewExplorer(t)
					mock.EXPECT().GetNode(tmock.Anything, "test-node").Return(nil, errors.New("error")).Maybe()
					return mock
				},
				node: &types.Node{
					Name:     "test-node",
					Instance: "test-instance",
					Region:   "test-region",
					Zone:     "test-zone",
				},
				cfg: &config.Config{
					WaitForNodeAddress: true,
					Filter:             []string{"test-filter"},
					OrderBy:            "test-order-by",
					RetryAttempts:      3,
					RetryInterval:      15 * time.Millisecond,
					LeaseDuration:      1,
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := prepareLogger("debug", false)
			explorer := tt.args.explorerFn(t)
			err := waitForAddressToBeReported(tt.args.c, log, explorer, tt.args.node, tt.args.address, tt.args.cfg)
			if err != nil != tt.wantErr {
				t.Errorf("waitForAddressToBeReported() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_waitForAddressDisabled(t *testing.T) {
	// No explorer is needed when the cloud assignment is enough.
	err := waitForAddressToBeReported(context.Background(), prepareLogger("error", false), nil, nil, "1.1.1.1", &config.Config{WaitForNodeAddress: false})
	if err != nil {
		t.Fatal(err)
	}
}

func Test_finalAttemptDoesNotWait(t *testing.T) {
	for _, assignment := range []bool{false, true} {
		t.Run(fmt.Sprint(assignment), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			cfg := &config.Config{WaitForNodeAddress: true, RetryAttempts: 0, RetryInterval: time.Hour, LeaseDuration: 5}
			n := &types.Node{Name: "test-node", Instance: "instance", Zone: "zone"}
			log := prepareLogger("error", false)
			var err error
			if assignment {
				assigner := mocks.NewAssigner(t)
				assigner.EXPECT().Assign(tmock.Anything, "instance", "zone", []string(nil), "").Return("", errors.New("no addresses")).Once()
				_, err = assignAddress(ctx, log, fake.NewSimpleClientset(), assigner, n, cfg)
			} else {
				explorer := nodeMocks.NewExplorer(t)
				explorer.EXPECT().GetNode(tmock.Anything, n.Name).Return(&types.Node{}, nil).Once()
				err = waitForAddressToBeReported(ctx, log, explorer, n, "1.1.1.1", cfg)
			}
			if err == nil || !strings.HasPrefix(err.Error(), "reached maximum number of retries") {
				t.Fatalf("expected exhausted retries without waiting, got %v", err)
			}
		})
	}
}
