package node

import (
	"context"
	"reflect"
	"testing"

	"github.com/doitintl/kubeip/internal/types"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func Test_deleteTaintsByKey(t *testing.T) {
	tests := []struct {
		name          string
		taints        []v1.Taint
		taintKey      string
		want          []v1.Taint
		wantDidDelete bool
	}{
		{
			name: "taints contains taintKey",
			taints: []v1.Taint{
				{
					Key:   "taint1",
					Value: "one",
				},
				{
					Key:   "taint2",
					Value: "two",
				},
			},
			taintKey: "taint2",
			want: []v1.Taint{
				{
					Key:   "taint1",
					Value: "one",
				},
			},
			wantDidDelete: true,
		},
		{
			name: "taint does not contain taintKey",
			taints: []v1.Taint{
				{
					Key:   "taint1",
					Value: "one",
				},
			},
			taintKey: "taint2",
			want: []v1.Taint{
				{
					Key:   "taint1",
					Value: "one",
				},
			},
			wantDidDelete: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotDidDelete := deleteTaintsByKey(tt.taints, tt.taintKey)

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("deleteTaintsByKey() got = %v, want %v", got, tt.want)
			}

			if gotDidDelete != tt.wantDidDelete {
				t.Errorf("deleteTaintsByKey() gotDidDelete = %v, want %v", gotDidDelete, tt.wantDidDelete)
			}
		})
	}
}

func Test_tainter_RemoveTaintKey(t *testing.T) {
	type fields struct {
		client *fake.Clientset
	}
	type args struct {
		node     *types.Node
		taintKey string
	}

	tests := []struct {
		name         string
		fields       *fields
		args         args
		want         bool
		wantErr      bool
		validateNode func(t *testing.T, node *v1.Node)
	}{
		{
			name: "remove taint key",
			fields: &fields{
				client: fake.NewSimpleClientset(&v1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
					},
					Spec: v1.NodeSpec{
						Taints: []v1.Taint{
							{
								Key:    "taint1",
								Value:  "true",
								Effect: "NoSchedule",
							},
							{
								Key:    "taint2",
								Value:  "two",
								Effect: "NoSchedule",
							},
						},
					},
				}),
			},
			args: args{
				node:     &types.Node{Name: "node1"},
				taintKey: "taint1",
			},
			want:    true,
			wantErr: false,
			validateNode: func(t *testing.T, node *v1.Node) {
				if node.ObjectMeta.Name != "node1" {
					t.Errorf("RemoveTaintKey() node.ObjectMeta.Name = %v, want node1", node.ObjectMeta.Name)
				}

				if len(node.Spec.Taints) != 1 {
					t.Errorf("RemoveTaintKey() node.Spec.Taints = %v, want 1", node.Spec.Taints)
				}

				if node.Spec.Taints[0].Key != "taint2" {
					t.Errorf("RemoveTaintKey() node.Spec.Taints[0].Key = %v, want taint2", node.Spec.Taints[0].Key)
				}
			},
		},
		{
			name: "only one taint key on node",
			fields: &fields{
				client: fake.NewSimpleClientset(&v1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
					},
					Spec: v1.NodeSpec{
						Taints: []v1.Taint{
							{
								Key:    "taint1",
								Value:  "true",
								Effect: "NoSchedule",
							},
						},
					},
				}),
			},
			args: args{
				node:     &types.Node{Name: "node1"},
				taintKey: "taint1",
			},
			want:    true,
			wantErr: false,
			validateNode: func(t *testing.T, node *v1.Node) {
				if node.ObjectMeta.Name != "node1" {
					t.Errorf("RemoveTaintKey() node.ObjectMeta.Name = %v, want node1", node.ObjectMeta.Name)
				}

				if len(node.Spec.Taints) != 0 {
					t.Errorf("RemoveTaintKey() node.Spec.Taints = %v, want 0", node.Spec.Taints)
				}
			},
		},
		{
			name: "taint key not present on node",
			fields: &fields{
				client: fake.NewSimpleClientset(&v1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
					},
					Spec: v1.NodeSpec{
						Taints: []v1.Taint{
							{
								Key:    "taint1",
								Value:  "true",
								Effect: "NoSchedule",
							},
						},
					},
				}),
			},
			args: args{
				node:     &types.Node{Name: "node1"},
				taintKey: "taint2",
			},
			want:    false,
			wantErr: false,
			validateNode: func(t *testing.T, node *v1.Node) {
				if node.ObjectMeta.Name != "node1" {
					t.Errorf("RemoveTaintKey() node.ObjectMeta.Name = %v, want node1", node.ObjectMeta.Name)
				}

				if len(node.Spec.Taints) != 1 {
					t.Errorf("RemoveTaintKey() node.Spec.Taints = %v, want 1", node.Spec.Taints)
				}

				if node.Spec.Taints[0].Key != "taint1" {
					t.Errorf("RemoveTaintKey() node.Spec.Taints[0].Key = %v, want taint1", node.Spec.Taints[0].Key)
				}
			},
		},
		{
			name: "no taints on node",
			fields: &fields{
				client: fake.NewSimpleClientset(&v1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
					},
					Spec: v1.NodeSpec{},
				}),
			},
			args: args{
				node:     &types.Node{Name: "node1"},
				taintKey: "taint1",
			},
			want:    false,
			wantErr: false,
			validateNode: func(t *testing.T, node *v1.Node) {
				if node.ObjectMeta.Name != "node1" {
					t.Errorf("RemoveTaintKey() node.ObjectMeta.Name = %v, want node1", node.ObjectMeta.Name)
				}

				if len(node.Spec.Taints) != 0 {
					t.Errorf("RemoveTaintKey() node.Spec.Taints = %v, want 0", node.Spec.Taints)
				}
			},
		},
		{
			name: "node not found",
			fields: &fields{
				client: fake.NewSimpleClientset(),
			},
			args: args{
				node:     &types.Node{Name: "node1"},
				taintKey: "taint1",
			},
			want:    false,
			wantErr: true,
			validateNode: func(t *testing.T, node *v1.Node) {
				// no node to validate
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			tainter := NewTainter(tt.fields.client)
			got, err := tainter.RemoveTaintKey(ctx, tt.args.node, tt.args.taintKey)

			if (err != nil) != tt.wantErr {
				t.Errorf("RemoveTaintKey() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("RemoveTaintKey() got = %v, want %v", got, tt.want)
			}

			if !tt.wantErr {
				node, _ := tt.fields.client.CoreV1().Nodes().Get(ctx, tt.args.node.Name, metav1.GetOptions{})
				tt.validateNode(t, node)
			}
		})
	}
}
