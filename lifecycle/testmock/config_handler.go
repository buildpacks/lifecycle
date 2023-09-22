// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/buildpacks/lifecycle/lifecycle (interfaces: ConfigHandler)

// Package testmock is a generated GoMock package.
package testmock

import (
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"

	buildpack "github.com/buildpacks/lifecycle/buildpack"
	log "github.com/buildpacks/lifecycle/log"
	files "github.com/buildpacks/lifecycle/platform/files"
)

// MockConfigHandler is a mock of ConfigHandler interface.
type MockConfigHandler struct {
	ctrl     *gomock.Controller
	recorder *MockConfigHandlerMockRecorder
}

// MockConfigHandlerMockRecorder is the mock recorder for MockConfigHandler.
type MockConfigHandlerMockRecorder struct {
	mock *MockConfigHandler
}

// NewMockConfigHandler creates a new mock instance.
func NewMockConfigHandler(ctrl *gomock.Controller) *MockConfigHandler {
	mock := &MockConfigHandler{ctrl: ctrl}
	mock.recorder = &MockConfigHandlerMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockConfigHandler) EXPECT() *MockConfigHandlerMockRecorder {
	return m.recorder
}

// ReadAnalyzed mocks base method.
func (m *MockConfigHandler) ReadAnalyzed(arg0 string, arg1 log.Logger) (files.Analyzed, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ReadAnalyzed", arg0, arg1)
	ret0, _ := ret[0].(files.Analyzed)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ReadAnalyzed indicates an expected call of ReadAnalyzed.
func (mr *MockConfigHandlerMockRecorder) ReadAnalyzed(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ReadAnalyzed", reflect.TypeOf((*MockConfigHandler)(nil).ReadAnalyzed), arg0, arg1)
}

// ReadGroup mocks base method.
func (m *MockConfigHandler) ReadGroup(arg0 string) ([]buildpack.GroupElement, []buildpack.GroupElement, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ReadGroup", arg0)
	ret0, _ := ret[0].([]buildpack.GroupElement)
	ret1, _ := ret[1].([]buildpack.GroupElement)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// ReadGroup indicates an expected call of ReadGroup.
func (mr *MockConfigHandlerMockRecorder) ReadGroup(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ReadGroup", reflect.TypeOf((*MockConfigHandler)(nil).ReadGroup), arg0)
}

// ReadOrder mocks base method.
func (m *MockConfigHandler) ReadOrder(arg0 string) (buildpack.Order, buildpack.Order, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ReadOrder", arg0)
	ret0, _ := ret[0].(buildpack.Order)
	ret1, _ := ret[1].(buildpack.Order)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// ReadOrder indicates an expected call of ReadOrder.
func (mr *MockConfigHandlerMockRecorder) ReadOrder(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ReadOrder", reflect.TypeOf((*MockConfigHandler)(nil).ReadOrder), arg0)
}

// ReadRun mocks base method.
func (m *MockConfigHandler) ReadRun(arg0 string, arg1 log.Logger) (files.Run, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ReadRun", arg0, arg1)
	ret0, _ := ret[0].(files.Run)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ReadRun indicates an expected call of ReadRun.
func (mr *MockConfigHandlerMockRecorder) ReadRun(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ReadRun", reflect.TypeOf((*MockConfigHandler)(nil).ReadRun), arg0, arg1)
}
