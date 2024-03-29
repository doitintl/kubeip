package lease

import (
	"context"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/ptr"
)

type KubeLock interface {
	Lock(ctx context.Context) error
	Unlock(ctx context.Context) error
}

type kubeLeaseLock struct {
	client         kubernetes.Interface
	leaseName      string
	namespace      string
	holderIdentity string
	leaseDuration  int // seconds
	cancelFunc     context.CancelFunc
}

func NewKubeLeaseLock(client kubernetes.Interface, leaseName, namespace, holderIdentity string, leaseDurationSeconds int) KubeLock {
	return &kubeLeaseLock{
		client:         client,
		leaseName:      leaseName,
		namespace:      namespace,
		holderIdentity: holderIdentity,
		leaseDuration:  leaseDurationSeconds,
	}
}

func (k *kubeLeaseLock) Lock(ctx context.Context) error {
	backoff := wait.Backoff{
		Duration: time.Second, // start with 1 second
		Factor:   1.5,         //nolint:gomnd // multiply by 1.5 on each retry
		Jitter:   0.5,         //nolint:gomnd // add 50% jitter to wait time on each retry
		Steps:    100,         //nolint:gomnd // retry 100 times
		Cap:      time.Hour,   // but never wait more than 1 hour
	}

	return wait.ExponentialBackoff(backoff, func() (bool, error) { //nolint:wrapcheck
		timestamp := metav1.MicroTime{Time: time.Now()}
		lease := &coordinationv1.Lease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      k.leaseName,
				Namespace: k.namespace,
			},
			Spec: coordinationv1.LeaseSpec{
				HolderIdentity:       &k.holderIdentity,
				LeaseDurationSeconds: ptr.To(int32(k.leaseDuration)),
				AcquireTime:          &timestamp,
				RenewTime:            &timestamp,
			},
		}

		_, err := k.client.CoordinationV1().Leases(k.namespace).Create(ctx, lease, metav1.CreateOptions{})
		if err != nil {
			if errors.IsAlreadyExists(err) {
				// If the lease already exists, check if it's held by another holder
				existingLease, getErr := k.client.CoordinationV1().Leases(k.namespace).Get(ctx, k.leaseName, metav1.GetOptions{})
				if getErr != nil {
					return false, getErr //nolint:wrapcheck
				}
				// check if the lease is expired
				if existingLease.Spec.RenewTime != nil && time.Since(existingLease.Spec.RenewTime.Time) > time.Duration(k.leaseDuration)*time.Second {
					// If the lease is expired, delete it and retry
					delErr := k.client.CoordinationV1().Leases(k.namespace).Delete(ctx, k.leaseName, metav1.DeleteOptions{})
					if delErr != nil {
						return false, delErr //nolint:wrapcheck
					}
					return false, nil
				}
				// check if the lease is held by another holder
				if existingLease.Spec.HolderIdentity != nil && *existingLease.Spec.HolderIdentity != k.holderIdentity {
					// If the lease is held by another holder, return false to retry
					return false, nil
				}
				return true, nil
			}
			return false, err //nolint:wrapcheck
		}

		// Create a child context with cancellation
		ctx, k.cancelFunc = context.WithCancel(ctx)
		go k.renewLeasePeriodically(ctx)

		return true, nil
	})
}

func (k *kubeLeaseLock) renewLeasePeriodically(ctx context.Context) {
	// let's renew the lease every 1/2 of the lease duration; use milliseconds for ticker
	ticker := time.NewTicker(time.Duration(k.leaseDuration*500) * time.Millisecond) //nolint:gomnd
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			lease, err := k.client.CoordinationV1().Leases(k.namespace).Get(ctx, k.leaseName, metav1.GetOptions{})
			if err != nil || lease.Spec.HolderIdentity == nil || *lease.Spec.HolderIdentity != k.holderIdentity {
				return
			}

			lease.Spec.RenewTime = &metav1.MicroTime{Time: time.Now()}
			k.client.CoordinationV1().Leases(k.namespace).Update(ctx, lease, metav1.UpdateOptions{}) //nolint:errcheck
		case <-ctx.Done():
			// Exit the goroutine when the context is cancelled
			return
		}
	}
}

func (k *kubeLeaseLock) Unlock(ctx context.Context) error {
	// Call the cancel function to stop the lease renewal process
	if k.cancelFunc != nil {
		k.cancelFunc()
	}
	lease, err := k.client.CoordinationV1().Leases(k.namespace).Get(ctx, k.leaseName, metav1.GetOptions{})
	if err != nil {
		return err //nolint:wrapcheck
	}

	if lease.Spec.HolderIdentity == nil || *lease.Spec.HolderIdentity != k.holderIdentity {
		return nil
	}

	return k.client.CoordinationV1().Leases(k.namespace).Delete(ctx, k.leaseName, metav1.DeleteOptions{}) //nolint:wrapcheck
}
