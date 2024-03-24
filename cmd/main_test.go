package main

import (
	"context"
	"testing"
	"time"

	"github.com/doitintl/kubeip/internal/address"
	"github.com/doitintl/kubeip/internal/config"
	"github.com/doitintl/kubeip/internal/types"
	mocks "github.com/doitintl/kubeip/mocks/address"
	"github.com/pkg/errors"
	tmock "github.com/stretchr/testify/mock"
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
		wantErr bool
	}{
		{
			name: "assign address successfully",
			args: args{
				c: context.Background(),
				assignerFn: func(t *testing.T) address.Assigner {
					mock := mocks.NewAssigner(t)
					mock.EXPECT().Assign(tmock.Anything, "test-instance", "test-zone", []string{"test-filter"}, "test-order-by").Return(nil)
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
				},
			},
		},
		{
			name: "assign address after a few retries",
			args: args{
				c: context.Background(),
				assignerFn: func(t *testing.T) address.Assigner {
					mock := mocks.NewAssigner(t)
					mock.EXPECT().Assign(tmock.Anything, "test-instance", "test-zone", []string{"test-filter"}, "test-order-by").Return(errors.New("first error")).Once()
					mock.EXPECT().Assign(tmock.Anything, "test-instance", "test-zone", []string{"test-filter"}, "test-order-by").Return(errors.New("second error")).Once()
					mock.EXPECT().Assign(tmock.Anything, "test-instance", "test-zone", []string{"test-filter"}, "test-order-by").Return(nil).Once()
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
				},
			},
		},
		{
			name: "error after a few retries and reached maximum number of retries",
			args: args{
				c: context.Background(),
				assignerFn: func(t *testing.T) address.Assigner {
					mock := mocks.NewAssigner(t)
					mock.EXPECT().Assign(tmock.Anything, "test-instance", "test-zone", []string{"test-filter"}, "test-order-by").Return(errors.New("error")).Times(4)
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
					mock.EXPECT().Assign(tmock.Anything, "test-instance", "test-zone", []string{"test-filter"}, "test-order-by").Return(errors.New("error")).Maybe()
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
					mock.EXPECT().Assign(tmock.Anything, "test-instance", "test-zone", []string{"test-filter"}, "test-order-by").Return(errors.New("error")).Maybe()
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
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := prepareLogger("debug", false)
			assigner := tt.args.assignerFn(t)
			if err := assignAddress(tt.args.c, log, assigner, tt.args.node, tt.args.cfg); (err != nil) != tt.wantErr {
				t.Errorf("assignAddress() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
