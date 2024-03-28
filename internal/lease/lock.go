package lease

import (
	"context"
	"sync"
	"time"

	errs "github.com/pkg/errors"
	coordinationv1 "k8s.io/api/coordination/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	leaseDuration  int32
	mutex          sync.Mutex
	ticker         *time.Ticker
}

func NewKubeLeaseLock(client kubernetes.Interface, leaseName, namespace, holderIdentity string, leaseDurationSeconds int32) KubeLock {
	return &kubeLeaseLock{
		client:         client,
		leaseName:      leaseName,
		namespace:      namespace,
		holderIdentity: holderIdentity,
		leaseDuration:  leaseDurationSeconds,
	}
}

func (k *kubeLeaseLock) Lock(ctx context.Context) error {
	for {
		lease, err := k.getOrCreateLease(ctx)
		if err != nil {
			return err
		}

		if k.shouldAcquireLease(lease) {
			return k.acquireLease(ctx, lease)
		}

		time.Sleep(time.Duration(*lease.Spec.LeaseDurationSeconds) * time.Second)
	}
}

func (k *kubeLeaseLock) getOrCreateLease(ctx context.Context) (*coordinationv1.Lease, error) {
	lease, err := k.client.CoordinationV1().Leases(k.namespace).Get(ctx, k.leaseName, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		timestamp := time.Now()
		lease = &coordinationv1.Lease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      k.leaseName,
				Namespace: k.namespace,
			},
			Spec: coordinationv1.LeaseSpec{
				HolderIdentity:       ptr.To(k.holderIdentity),
				LeaseDurationSeconds: ptr.To(k.leaseDuration),
				AcquireTime:          &metav1.MicroTime{Time: timestamp},
				RenewTime:            &metav1.MicroTime{Time: timestamp},
			},
		}

		_, err = k.client.CoordinationV1().Leases(k.namespace).Create(ctx, lease, metav1.CreateOptions{})
	}

	return lease, err //nolint:wrapcheck
}

func (k *kubeLeaseLock) shouldAcquireLease(lease *coordinationv1.Lease) bool {
	return lease.Spec.HolderIdentity == nil || time.Since(lease.Spec.RenewTime.Time) > time.Duration(*lease.Spec.LeaseDurationSeconds)*time.Second
}

func (k *kubeLeaseLock) acquireLease(ctx context.Context, lease *coordinationv1.Lease) error {
	k.mutex.Lock()
	defer k.mutex.Unlock()

	lease.Spec.HolderIdentity = ptr.To(k.holderIdentity)
	lease.Spec.RenewTime = &metav1.MicroTime{Time: time.Now()}

	_, err := k.client.CoordinationV1().Leases(k.namespace).Update(ctx, lease, metav1.UpdateOptions{})
	if err != nil {
		return err //nolint:wrapcheck
	}

	k.ticker = time.NewTicker(time.Duration(*lease.Spec.LeaseDurationSeconds) * time.Second)
	go k.renewLeasePeriodically(ctx, lease)

	return nil
}

func (k *kubeLeaseLock) renewLeasePeriodically(ctx context.Context, lease *coordinationv1.Lease) {
	if k == nil || k.ticker == nil {
		return
	}
	for range k.ticker.C {
		k.mutex.Lock()
		lease.Spec.RenewTime = &metav1.MicroTime{Time: time.Now()}
		k.client.CoordinationV1().Leases(k.namespace).Update(ctx, lease, metav1.UpdateOptions{}) //nolint:errcheck
		k.mutex.Unlock()
	}
}

func (k *kubeLeaseLock) Unlock(ctx context.Context) error {
	k.mutex.Lock()
	defer k.mutex.Unlock()

	lease, err := k.client.CoordinationV1().Leases(k.namespace).Get(ctx, k.leaseName, metav1.GetOptions{})
	if err != nil {
		return err //nolint:wrapcheck
	}

	if lease.Spec.HolderIdentity == nil || *lease.Spec.HolderIdentity != k.holderIdentity {
		return errs.New("lease is not held by the current holderIdentity")
	}

	lease.Spec.HolderIdentity = nil

	_, err = k.client.CoordinationV1().Leases(k.namespace).Update(ctx, lease, metav1.UpdateOptions{})
	if err != nil {
		return err //nolint:wrapcheck
	}

	if k.ticker != nil {
		k.ticker.Stop()
		k.ticker = nil
	}

	return nil
}
