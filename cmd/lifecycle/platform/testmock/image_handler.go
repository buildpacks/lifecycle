// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/buildpacks/lifecycle/cmd/lifecycle/platform (interfaces: ImageHandler)

// Package testmock is a generated GoMock package.
package testmock

import (
	reflect "reflect"

	imgutil "github.com/buildpacks/imgutil"
	gomock "github.com/golang/mock/gomock"
	authn "github.com/google/go-containerregistry/pkg/authn"
)

// MockImageHandler is a mock of ImageHandler interface.
type MockImageHandler struct {
	ctrl     *gomock.Controller
	recorder *MockImageHandlerMockRecorder
}

// MockImageHandlerMockRecorder is the mock recorder for MockImageHandler.
type MockImageHandlerMockRecorder struct {
	mock *MockImageHandler
}

// NewMockImageHandler creates a new mock instance.
func NewMockImageHandler(ctrl *gomock.Controller) *MockImageHandler {
	mock := &MockImageHandler{ctrl: ctrl}
	mock.recorder = &MockImageHandlerMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockImageHandler) EXPECT() *MockImageHandlerMockRecorder {
	return m.recorder
}

// Docker mocks base method.
func (m *MockImageHandler) Docker() bool {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Docker")
	ret0, _ := ret[0].(bool)
	return ret0
}

// Docker indicates an expected call of Docker.
func (mr *MockImageHandlerMockRecorder) Docker() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Docker", reflect.TypeOf((*MockImageHandler)(nil).Docker))
}

// InitImage mocks base method.
func (m *MockImageHandler) InitImage(arg0 string) (imgutil.Image, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "InitImage", arg0)
	ret0, _ := ret[0].(imgutil.Image)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// InitImage indicates an expected call of InitImage.
func (mr *MockImageHandlerMockRecorder) InitImage(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "InitImage", reflect.TypeOf((*MockImageHandler)(nil).InitImage), arg0)
}

// Keychain mocks base method.
func (m *MockImageHandler) Keychain() authn.Keychain {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Keychain")
	ret0, _ := ret[0].(authn.Keychain)
	return ret0
}

// Keychain indicates an expected call of Keychain.
func (mr *MockImageHandlerMockRecorder) Keychain() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Keychain", reflect.TypeOf((*MockImageHandler)(nil).Keychain))
}
