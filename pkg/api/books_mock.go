// Code generated by MockGen. DO NOT EDIT.
// Source: pkg/api/books.go

// Package api is a generated GoMock package.
package api

import (
	reflect "reflect"

	gin "github.com/gin-gonic/gin"
	gomock "github.com/golang/mock/gomock"
)

// MockBookRepository is a mock of BookRepository interface.
type MockBookRepository struct {
	ctrl     *gomock.Controller
	recorder *MockBookRepositoryMockRecorder
}

// MockBookRepositoryMockRecorder is the mock recorder for MockBookRepository.
type MockBookRepositoryMockRecorder struct {
	mock *MockBookRepository
}

// NewMockBookRepository creates a new mock instance.
func NewMockBookRepository(ctrl *gomock.Controller) *MockBookRepository {
	mock := &MockBookRepository{ctrl: ctrl}
	mock.recorder = &MockBookRepositoryMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockBookRepository) EXPECT() *MockBookRepositoryMockRecorder {
	return m.recorder
}

// CreateBook mocks base method.
func (m *MockBookRepository) CreateBook(c *gin.Context) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "CreateBook", c)
}

// CreateBook indicates an expected call of CreateBook.
func (mr *MockBookRepositoryMockRecorder) CreateBook(c interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CreateBook", reflect.TypeOf((*MockBookRepository)(nil).CreateBook), c)
}

// DeleteBook mocks base method.
func (m *MockBookRepository) DeleteBook(c *gin.Context) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "DeleteBook", c)
}

// DeleteBook indicates an expected call of DeleteBook.
func (mr *MockBookRepositoryMockRecorder) DeleteBook(c interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DeleteBook", reflect.TypeOf((*MockBookRepository)(nil).DeleteBook), c)
}

// FindBook mocks base method.
func (m *MockBookRepository) FindBook(c *gin.Context) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "FindBook", c)
}

// FindBook indicates an expected call of FindBook.
func (mr *MockBookRepositoryMockRecorder) FindBook(c interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "FindBook", reflect.TypeOf((*MockBookRepository)(nil).FindBook), c)
}

// FindBooks mocks base method.
func (m *MockBookRepository) FindBooks(c *gin.Context) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "FindBooks", c)
}

// FindBooks indicates an expected call of FindBooks.
func (mr *MockBookRepositoryMockRecorder) FindBooks(c interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "FindBooks", reflect.TypeOf((*MockBookRepository)(nil).FindBooks), c)
}

// Healthcheck mocks base method.
func (m *MockBookRepository) Healthcheck(c *gin.Context) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "Healthcheck", c)
}

// Healthcheck indicates an expected call of Healthcheck.
func (mr *MockBookRepositoryMockRecorder) Healthcheck(c interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Healthcheck", reflect.TypeOf((*MockBookRepository)(nil).Healthcheck), c)
}

// UpdateBook mocks base method.
func (m *MockBookRepository) UpdateBook(c *gin.Context) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "UpdateBook", c)
}

// UpdateBook indicates an expected call of UpdateBook.
func (mr *MockBookRepositoryMockRecorder) UpdateBook(c interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpdateBook", reflect.TypeOf((*MockBookRepository)(nil).UpdateBook), c)
}