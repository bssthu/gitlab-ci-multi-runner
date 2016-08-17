package service_helpers

import (
	"errors"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/bssthu/gitlab-ci-multi-runner/mocks"
	"testing"
)

var errExample = errors.New("example error")

func TestStart(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mi := &mocks.Interface{}
	s := &SimpleService{i: mi}

	mi.On("Start", s).Return(errExample)

	err := s.Run()
	assert.Equal(t, err, errExample)
	mi.AssertExpectations(t)
}
