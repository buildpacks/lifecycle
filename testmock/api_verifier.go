// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/buildpacks/lifecycle (interfaces: APIVerifier)

// Package testmock is a generated GoMock package.
package testmock

import (
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
)

// MockAPIVerifier is a mock of APIVerifier interface.
type MockAPIVerifier struct {
	ctrl     *gomock.Controller
	recorder *MockAPIVerifierMockRecorder
}

// MockAPIVerifierMockRecorder is the mock recorder for MockAPIVerifier.
type MockAPIVerifierMockRecorder struct {
	mock *MockAPIVerifier
}

// NewMockAPIVerifier creates a new mock instance.
func NewMockAPIVerifier(ctrl *gomock.Controller) *MockAPIVerifier {
	mock := &MockAPIVerifier{ctrl: ctrl}
	mock.recorder = &MockAPIVerifierMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockAPIVerifier) EXPECT() *MockAPIVerifierMockRecorder {
	return m.recorder
}

// VerifyBuildpackAPIForBuildpack mocks base method.
func (m *MockAPIVerifier) VerifyBuildpackAPIForBuildpack(arg0, arg1 string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "VerifyBuildpackAPIForBuildpack", arg0, arg1)
	ret0, _ := ret[0].(error)
	return ret0
}

// VerifyBuildpackAPIForBuildpack indicates an expected call of VerifyBuildpackAPIForBuildpack.
func (mr *MockAPIVerifierMockRecorder) VerifyBuildpackAPIForBuildpack(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "VerifyBuildpackAPIForBuildpack", reflect.TypeOf((*MockAPIVerifier)(nil).VerifyBuildpackAPIForBuildpack), arg0, arg1)
}

// VerifyBuildpackAPIForExtension mocks base method.
func (m *MockAPIVerifier) VerifyBuildpackAPIForExtension(arg0, arg1 string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "VerifyBuildpackAPIForExtension", arg0, arg1)
	ret0, _ := ret[0].(error)
	return ret0
}

// VerifyBuildpackAPIForExtension indicates an expected call of VerifyBuildpackAPIForExtension.
func (mr *MockAPIVerifierMockRecorder) VerifyBuildpackAPIForExtension(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "VerifyBuildpackAPIForExtension", reflect.TypeOf((*MockAPIVerifier)(nil).VerifyBuildpackAPIForExtension), arg0, arg1)
}
