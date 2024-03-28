package lease

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/utils/ptr"
)

func TestLockAndUnlock(t *testing.T) {
	tests := []struct {
		name               string
		leaseExists        bool
		holderIdentity     string
		leaseCurrentHolder *string
		leaseDuration      int32
		skipLock           bool
		expectLockErr      bool
		expectUnlockErr    bool
	}{
		{
			name:            "Lock acquires lease when none exists",
			holderIdentity:  "test-holder",
			leaseExists:     false,
			leaseDuration:   1,
			expectLockErr:   false,
			expectUnlockErr: false,
		},
		{
			name:               "Lock acquires lease when held by another and expires",
			leaseExists:        true,
			holderIdentity:     "test-holder",
			leaseCurrentHolder: ptr.To("another-holder"),
			leaseDuration:      1,
			expectLockErr:      false,
			expectUnlockErr:    false,
		},
		{
			name:               "Unlock releases lease",
			leaseExists:        true,
			holderIdentity:     "test-holder",
			leaseCurrentHolder: ptr.To("test-holder"),
			leaseDuration:      1,
			expectLockErr:      false,
			expectUnlockErr:    false,
		},
		{
			name:               "Unlock does not release lease when locked by another holder",
			leaseExists:        true,
			leaseCurrentHolder: ptr.To("another-holder"),
			holderIdentity:     "test-holder",
			leaseDuration:      1,
			skipLock:           true,
			expectLockErr:      true,
			expectUnlockErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := fake.NewSimpleClientset()
			if tt.leaseExists {
				lease := &v1.Lease{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-lease",
						Namespace: "test-namespace",
					},
					Spec: v1.LeaseSpec{
						HolderIdentity:       tt.leaseCurrentHolder,
						LeaseDurationSeconds: ptr.To(int32(1)),
						AcquireTime:          &metav1.MicroTime{Time: time.Now()},
						RenewTime:            &metav1.MicroTime{Time: time.Now()},
					},
				}

				_, err := client.CoordinationV1().Leases("test-namespace").Create(context.Background(), lease, metav1.CreateOptions{})
				require.NoError(t, err)
			}

			lock := NewKubeLeaseLock(client, "test-lease", "test-namespace", tt.holderIdentity, tt.leaseDuration)

			if !tt.skipLock {
				err := lock.Lock(context.Background())
				if tt.expectLockErr {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
			}

			err := lock.Unlock(context.Background())
			if tt.expectUnlockErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
