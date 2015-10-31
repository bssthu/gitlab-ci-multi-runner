package service_helpers

import (
	"testing"
	"github.com/golang/mock/gomock"
	"github.com/bssthu/gitlab-ci-multi-runner/mocks"
	"errors"
	"github.com/stretchr/testify/assert"
)

var ExampleError = errors.New("example error")

func TestStart(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mi := &mocks.Interface{}
	s := &SimpleService{i: mi}

	mi.On("Start", s).Return(ExampleError)

	err := s.Run()
	assert.Equal(t, err, ExampleError)
	mi.AssertExpectations(t)
}
