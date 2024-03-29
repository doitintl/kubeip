package lease

import (
	"context"
	"fmt"
	"sync"
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
		leaseDuration      int
		skipLock           bool
		expectLockErr      bool
		expectUnlockErr    bool
	}{
		{
			name:           "Lock acquires lease when none exists",
			holderIdentity: "test-holder",
			leaseExists:    false,
			leaseDuration:  1,
		},
		{
			name:               "Lock acquires lease when held by another and expires",
			leaseExists:        true,
			holderIdentity:     "test-holder",
			leaseCurrentHolder: ptr.To("another-holder"),
			leaseDuration:      2,
		},
		{
			name:               "Unlock releases lease",
			leaseExists:        true,
			holderIdentity:     "test-holder",
			leaseCurrentHolder: ptr.To("test-holder"),
			leaseDuration:      1,
		},
		{
			name:               "Unlock does not release lease when locked by another holder",
			leaseExists:        true,
			leaseCurrentHolder: ptr.To("another-holder"),
			holderIdentity:     "test-holder",
			leaseDuration:      1,
			skipLock:           true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := fake.NewSimpleClientset()
			if tt.leaseExists {
				timestamp := metav1.MicroTime{Time: time.Now()}
				lease := &v1.Lease{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-lease",
						Namespace: "test-namespace",
					},
					Spec: v1.LeaseSpec{
						HolderIdentity:       tt.leaseCurrentHolder,
						LeaseDurationSeconds: ptr.To(int32(1)),
						AcquireTime:          &timestamp,
						RenewTime:            &timestamp,
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

func TestConcurrentLock(t *testing.T) {
	client := fake.NewSimpleClientset()

	var wg sync.WaitGroup
	wg.Add(2)

	// Goroutine 1: Acquire the lock and hold it for 5 seconds
	go func() {
		defer wg.Done()
		lock := NewKubeLeaseLock(client, "test-lease", "test-namespace", "test-holder-1", 5)
		err := lock.Lock(context.Background())
		assert.NoError(t, err)
		fmt.Println("Lock acquired by goroutine 1")
		time.Sleep(2 * time.Second)
		err = lock.Unlock(context.Background())
		assert.NoError(t, err)
		fmt.Println("Lock released by goroutine 1")
	}()

	time.Sleep(100 * time.Millisecond)

	// Goroutine 2: Try to acquire the lock and wait until it succeeds
	go func() {
		defer wg.Done()
		lock := NewKubeLeaseLock(client, "test-lease", "test-namespace", "test-holder-2", 5)
		err := lock.Lock(context.Background())
		assert.NoError(t, err)
		fmt.Println("Lock acquired by goroutine 2")
		err = lock.Unlock(context.Background())
		assert.NoError(t, err)
		fmt.Println("Lock released by goroutine 2")
	}()

	wg.Wait()
}
