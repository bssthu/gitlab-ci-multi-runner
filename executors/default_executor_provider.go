package executors

import "github.com/bssthu/gitlab-ci-multi-runner/common"

type DefaultExecutorProvider struct {
	Creator         func() common.Executor
	FeaturesUpdater func(features *common.FeaturesInfo)
}

func (e DefaultExecutorProvider) CanCreate() bool {
	return e.Creator != nil
}

func (e DefaultExecutorProvider) Create() common.Executor {
	if e.Creator == nil {
		return nil
	}
	return e.Creator()
}

func (e DefaultExecutorProvider) Acquire(config *common.RunnerConfig) (common.ExecutorData, error) {
	return nil, nil
}

func (e DefaultExecutorProvider) Release(config *common.RunnerConfig, data common.ExecutorData) error {
	return nil
}

func (e DefaultExecutorProvider) GetFeatures(features *common.FeaturesInfo) {
	if e.FeaturesUpdater != nil {
		e.FeaturesUpdater(features)
	}
}
