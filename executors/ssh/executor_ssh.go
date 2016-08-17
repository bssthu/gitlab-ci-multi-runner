package ssh

import (
	"errors"

	"github.com/bssthu/gitlab-ci-multi-runner/common"
	"github.com/bssthu/gitlab-ci-multi-runner/executors"
	"github.com/bssthu/gitlab-ci-multi-runner/helpers/ssh"
)

type executor struct {
	executors.AbstractExecutor
	sshCommand ssh.Client
}

func (s *executor) Prepare(globalConfig *common.Config, config *common.RunnerConfig, build *common.Build) error {
	err := s.AbstractExecutor.Prepare(globalConfig, config, build)
	if err != nil {
		return err
	}

	s.Println("Using SSH executor...")
	if s.BuildShell.PassFile {
		return errors.New("SSH doesn't support shells that require script file")
	}

	if s.Config.SSH == nil {
		return errors.New("Missing SSH configuration")
	}

	s.Debugln("Starting SSH command...")

	// Create SSH command
	s.sshCommand = ssh.Client{
		Config: *s.Config.SSH,
		Stdout: s.BuildTrace,
		Stderr: s.BuildTrace,
	}

	s.Debugln("Connecting to SSH server...")
	err = s.sshCommand.Connect()
	if err != nil {
		return err
	}
	return nil
}

func (s *executor) Run(cmd common.ExecutorCommand) error {
	err := s.sshCommand.Run(ssh.Command{
		Environment: s.BuildShell.Environment,
		Command:     s.BuildShell.GetCommandWithArguments(),
		Stdin:       cmd.Script,
		Abort:       cmd.Abort,
	})
	if _, ok := err.(*ssh.ExitError); ok {
		err = &common.BuildError{Inner: err}
	}
	return err
}

func (s *executor) Cleanup() {
	s.sshCommand.Cleanup()
	s.AbstractExecutor.Cleanup()
}

func init() {
	options := executors.ExecutorOptions{
		DefaultBuildsDir: "builds",
		SharedBuildsDir:  true,
		Shell: common.ShellScriptInfo{
			Shell:         "bash",
			Type:          common.LoginShell,
			RunnerCommand: "gitlab-runner",
		},
		ShowHostname: true,
	}

	creator := func() common.Executor {
		return &executor{
			AbstractExecutor: executors.AbstractExecutor{
				ExecutorOptions: options,
			},
		}
	}

	featuresUpdater := func(features *common.FeaturesInfo) {
		features.Variables = true
	}

	common.RegisterExecutor("ssh", executors.DefaultExecutorProvider{
		Creator:         creator,
		FeaturesUpdater: featuresUpdater,
	})
}
