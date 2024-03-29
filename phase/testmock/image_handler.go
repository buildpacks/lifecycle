// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/buildpacks/lifecycle/image (interfaces: Handler)

// Package testmock is a generated GoMock package.
package testmock

import (
	reflect "reflect"

	imgutil "github.com/buildpacks/imgutil"
	gomock "github.com/golang/mock/gomock"
)

// MockHandler is a mock of Handler interface.
type MockHandler struct {
	ctrl     *gomock.Controller
	recorder *MockHandlerMockRecorder
}

// MockHandlerMockRecorder is the mock recorder for MockHandler.
type MockHandlerMockRecorder struct {
	mock *MockHandler
}

// NewMockHandler creates a new mock instance.
func NewMockHandler(ctrl *gomock.Controller) *MockHandler {
	mock := &MockHandler{ctrl: ctrl}
	mock.recorder = &MockHandlerMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockHandler) EXPECT() *MockHandlerMockRecorder {
	return m.recorder
}

// InitImage mocks base method.
func (m *MockHandler) InitImage(arg0 string) (imgutil.Image, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "InitImage", arg0)
	ret0, _ := ret[0].(imgutil.Image)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// InitImage indicates an expected call of InitImage.
func (mr *MockHandlerMockRecorder) InitImage(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "InitImage", reflect.TypeOf((*MockHandler)(nil).InitImage), arg0)
}

// Kind mocks base method.
func (m *MockHandler) Kind() string {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Kind")
	ret0, _ := ret[0].(string)
	return ret0
}

// Kind indicates an expected call of Kind.
func (mr *MockHandlerMockRecorder) Kind() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Kind", reflect.TypeOf((*MockHandler)(nil).Kind))
}
