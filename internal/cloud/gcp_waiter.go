package cloud

import (
	"context"

	"google.golang.org/api/compute/v1"
)

type WaitCall interface {
	Context(ctx context.Context) WaitCall
	Do() (*compute.Operation, error)
}

type ZoneWaiter interface {
	Wait(projectID, region, operationName string) WaitCall
}

type zoneWaiter struct {
	client *compute.Service
}

type zoneWaitCall struct {
	call *compute.ZoneOperationsWaitCall
}

func NewZoneWaiter(client *compute.Service) ZoneWaiter {
	return &zoneWaiter{client: client}
}

func (w *zoneWaiter) Wait(projectID, region, operationName string) WaitCall {
	return &zoneWaitCall{w.client.ZoneOperations.Wait(projectID, region, operationName)}
}

func (c *zoneWaitCall) Context(ctx context.Context) WaitCall {
	return &zoneWaitCall{c.call.Context(ctx)}
}

func (c *zoneWaitCall) Do() (*compute.Operation, error) {
	return c.call.Do() //nolint:wrapcheck
}
