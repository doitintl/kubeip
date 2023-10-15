// Code generated by mockery v2.30.16. DO NOT EDIT.

package mocks

import (
	cloud "github.com/doitintl/kubeip/internal/cloud"
	compute "google.golang.org/api/compute/v1"

	context "context"

	mock "github.com/stretchr/testify/mock"
)

// WaitCall is an autogenerated mock type for the WaitCall type
type WaitCall struct {
	mock.Mock
}

type WaitCall_Expecter struct {
	mock *mock.Mock
}

func (_m *WaitCall) EXPECT() *WaitCall_Expecter {
	return &WaitCall_Expecter{mock: &_m.Mock}
}

// Context provides a mock function with given fields: ctx
func (_m *WaitCall) Context(ctx context.Context) cloud.WaitCall {
	ret := _m.Called(ctx)

	var r0 cloud.WaitCall
	if rf, ok := ret.Get(0).(func(context.Context) cloud.WaitCall); ok {
		r0 = rf(ctx)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(cloud.WaitCall)
		}
	}

	return r0
}

// WaitCall_Context_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Context'
type WaitCall_Context_Call struct {
	*mock.Call
}

// Context is a helper method to define mock.On call
//   - ctx context.Context
func (_e *WaitCall_Expecter) Context(ctx interface{}) *WaitCall_Context_Call {
	return &WaitCall_Context_Call{Call: _e.mock.On("Context", ctx)}
}

func (_c *WaitCall_Context_Call) Run(run func(ctx context.Context)) *WaitCall_Context_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context))
	})
	return _c
}

func (_c *WaitCall_Context_Call) Return(_a0 cloud.WaitCall) *WaitCall_Context_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *WaitCall_Context_Call) RunAndReturn(run func(context.Context) cloud.WaitCall) *WaitCall_Context_Call {
	_c.Call.Return(run)
	return _c
}

// Do provides a mock function with given fields:
func (_m *WaitCall) Do() (*compute.Operation, error) {
	ret := _m.Called()

	var r0 *compute.Operation
	var r1 error
	if rf, ok := ret.Get(0).(func() (*compute.Operation, error)); ok {
		return rf()
	}
	if rf, ok := ret.Get(0).(func() *compute.Operation); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*compute.Operation)
		}
	}

	if rf, ok := ret.Get(1).(func() error); ok {
		r1 = rf()
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// WaitCall_Do_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Do'
type WaitCall_Do_Call struct {
	*mock.Call
}

// Do is a helper method to define mock.On call
func (_e *WaitCall_Expecter) Do() *WaitCall_Do_Call {
	return &WaitCall_Do_Call{Call: _e.mock.On("Do")}
}

func (_c *WaitCall_Do_Call) Run(run func()) *WaitCall_Do_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *WaitCall_Do_Call) Return(_a0 *compute.Operation, _a1 error) *WaitCall_Do_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *WaitCall_Do_Call) RunAndReturn(run func() (*compute.Operation, error)) *WaitCall_Do_Call {
	_c.Call.Return(run)
	return _c
}

// NewWaitCall creates a new instance of WaitCall. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewWaitCall(t interface {
	mock.TestingT
	Cleanup(func())
}) *WaitCall {
	mock := &WaitCall{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}