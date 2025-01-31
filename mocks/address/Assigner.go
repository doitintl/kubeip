// Code generated by mockery v2.35.2. DO NOT EDIT.

package mocks

import (
	context "context"

	mock "github.com/stretchr/testify/mock"
)

// Assigner is an autogenerated mock type for the Assigner type
type Assigner struct {
	mock.Mock
}

type Assigner_Expecter struct {
	mock *mock.Mock
}

func (_m *Assigner) EXPECT() *Assigner_Expecter {
	return &Assigner_Expecter{mock: &_m.Mock}
}

// Assign provides a mock function with given fields: ctx, instanceID, zone, filter, orderBy
func (_m *Assigner) Assign(ctx context.Context, instanceID string, zone string, filter []string, orderBy string) (string, error) {
	ret := _m.Called(ctx, instanceID, zone, filter, orderBy)

	var r0 string
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, string, string, []string, string) (string, error)); ok {
		return rf(ctx, instanceID, zone, filter, orderBy)
	}
	if rf, ok := ret.Get(0).(func(context.Context, string, string, []string, string) string); ok {
		r0 = rf(ctx, instanceID, zone, filter, orderBy)
	} else {
		r0 = ret.Get(0).(string)
	}

	if rf, ok := ret.Get(1).(func(context.Context, string, string, []string, string) error); ok {
		r1 = rf(ctx, instanceID, zone, filter, orderBy)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Assigner_Assign_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Assign'
type Assigner_Assign_Call struct {
	*mock.Call
}

// Assign is a helper method to define mock.On call
//   - ctx context.Context
//   - instanceID string
//   - zone string
//   - filter []string
//   - orderBy string
func (_e *Assigner_Expecter) Assign(ctx interface{}, instanceID interface{}, zone interface{}, filter interface{}, orderBy interface{}) *Assigner_Assign_Call {
	return &Assigner_Assign_Call{Call: _e.mock.On("Assign", ctx, instanceID, zone, filter, orderBy)}
}

func (_c *Assigner_Assign_Call) Run(run func(ctx context.Context, instanceID string, zone string, filter []string, orderBy string)) *Assigner_Assign_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(string), args[2].(string), args[3].([]string), args[4].(string))
	})
	return _c
}

func (_c *Assigner_Assign_Call) Return(_a0 string, _a1 error) *Assigner_Assign_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *Assigner_Assign_Call) RunAndReturn(run func(context.Context, string, string, []string, string) (string, error)) *Assigner_Assign_Call {
	_c.Call.Return(run)
	return _c
}

// Unassign provides a mock function with given fields: ctx, instanceID, zone
func (_m *Assigner) Unassign(ctx context.Context, instanceID string, zone string) error {
	ret := _m.Called(ctx, instanceID, zone)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, string, string) error); ok {
		r0 = rf(ctx, instanceID, zone)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Assigner_Unassign_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Unassign'
type Assigner_Unassign_Call struct {
	*mock.Call
}

// Unassign is a helper method to define mock.On call
//   - ctx context.Context
//   - instanceID string
//   - zone string
func (_e *Assigner_Expecter) Unassign(ctx interface{}, instanceID interface{}, zone interface{}) *Assigner_Unassign_Call {
	return &Assigner_Unassign_Call{Call: _e.mock.On("Unassign", ctx, instanceID, zone)}
}

func (_c *Assigner_Unassign_Call) Run(run func(ctx context.Context, instanceID string, zone string)) *Assigner_Unassign_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(string), args[2].(string))
	})
	return _c
}

func (_c *Assigner_Unassign_Call) Return(_a0 error) *Assigner_Unassign_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *Assigner_Unassign_Call) RunAndReturn(run func(context.Context, string, string) error) *Assigner_Unassign_Call {
	_c.Call.Return(run)
	return _c
}

// NewAssigner creates a new instance of Assigner. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewAssigner(t interface {
	mock.TestingT
	Cleanup(func())
}) *Assigner {
	mock := &Assigner{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
