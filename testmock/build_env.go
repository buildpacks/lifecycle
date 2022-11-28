// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/buildpacks/lifecycle (interfaces: BuildEnv)

// Package testmock is a generated GoMock package.
package testmock

import (
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"

	env "github.com/buildpacks/lifecycle/env"
)

// MockBuildEnv is a mock of BuildEnv interface.
type MockBuildEnv struct {
	ctrl     *gomock.Controller
	recorder *MockBuildEnvMockRecorder
}

// MockBuildEnvMockRecorder is the mock recorder for MockBuildEnv.
type MockBuildEnvMockRecorder struct {
	mock *MockBuildEnv
}

// NewMockBuildEnv creates a new mock instance.
func NewMockBuildEnv(ctrl *gomock.Controller) *MockBuildEnv {
	mock := &MockBuildEnv{ctrl: ctrl}
	mock.recorder = &MockBuildEnvMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockBuildEnv) EXPECT() *MockBuildEnvMockRecorder {
	return m.recorder
}

// AddEnvDir mocks base method.
func (m *MockBuildEnv) AddEnvDir(arg0 string, arg1 env.ActionType) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "AddEnvDir", arg0, arg1)
	ret0, _ := ret[0].(error)
	return ret0
}

// AddEnvDir indicates an expected call of AddEnvDir.
func (mr *MockBuildEnvMockRecorder) AddEnvDir(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "AddEnvDir", reflect.TypeOf((*MockBuildEnv)(nil).AddEnvDir), arg0, arg1)
}

// AddRootDir mocks base method.
func (m *MockBuildEnv) AddRootDir(arg0 string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "AddRootDir", arg0)
	ret0, _ := ret[0].(error)
	return ret0
}

// AddRootDir indicates an expected call of AddRootDir.
func (mr *MockBuildEnvMockRecorder) AddRootDir(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "AddRootDir", reflect.TypeOf((*MockBuildEnv)(nil).AddRootDir), arg0)
}

// List mocks base method.
func (m *MockBuildEnv) List() []string {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "List")
	ret0, _ := ret[0].([]string)
	return ret0
}

// List indicates an expected call of List.
func (mr *MockBuildEnvMockRecorder) List() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "List", reflect.TypeOf((*MockBuildEnv)(nil).List))
}

// WithOverrides mocks base method.
func (m *MockBuildEnv) WithOverrides(arg0, arg1 string) ([]string, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "WithOverrides", arg0, arg1)
	ret0, _ := ret[0].([]string)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// WithOverrides indicates an expected call of WithOverrides.
func (mr *MockBuildEnvMockRecorder) WithOverrides(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "WithOverrides", reflect.TypeOf((*MockBuildEnv)(nil).WithOverrides), arg0, arg1)
}
