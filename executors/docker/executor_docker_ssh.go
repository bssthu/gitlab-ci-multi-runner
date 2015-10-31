package docker

import (
	"errors"

	"github.com/bssthu/gitlab-ci-multi-runner/common"
	"github.com/bssthu/gitlab-ci-multi-runner/executors"
	"github.com/bssthu/gitlab-ci-multi-runner/helpers/ssh"
)

type DockerSSHExecutor struct {
	DockerExecutor
	sshCommand ssh.Command
}

func (s *DockerSSHExecutor) Start() error {
	if s.Config.SSH == nil {
		return errors.New("Missing SSH configuration")
	}

	s.Debugln("Starting SSH command...")

	// Create container
	err := s.createBuildContainer([]string{})
	if err != nil {
		return err
	}

	containerData, err := s.client.InspectContainer(s.buildContainer.ID)
	if err != nil {
		return err
	}

	// Create SSH command
	s.sshCommand = ssh.Command{
		Config:      *s.Config.SSH,
		Environment: s.ShellScript.Environment,
		Command:     s.ShellScript.GetFullCommand(),
		Stdin:       s.ShellScript.GetScriptBytes(),
		Stdout:      s.BuildLog,
		Stderr:      s.BuildLog,
	}
	s.sshCommand.Host = &containerData.NetworkSettings.IPAddress

	s.Debugln("Connecting to SSH server...")
	err = s.sshCommand.Connect()
	if err != nil {
		return err
	}

	// Wait for process to exit
	go func() {
		s.Debugln("Will run SSH command...")
		err := s.sshCommand.Run()
		s.Debugln("SSH command finished with", err)
		s.BuildFinish <- err
	}()
	return nil
}

func (s *DockerSSHExecutor) Cleanup() {
	s.sshCommand.Cleanup()
	s.DockerExecutor.Cleanup()
}

func init() {
	options := executors.ExecutorOptions{
		DefaultBuildsDir: "builds",
		SharedBuildsDir:  false,
		Shell: common.ShellScriptInfo{
			Shell: "bash",
			Type:  common.LoginShell,
		},
		ShowHostname:     true,
		SupportedOptions: []string{"image", "services"},
	}

	create := func() common.Executor {
		return &DockerSSHExecutor{
			DockerExecutor: DockerExecutor{
				AbstractExecutor: executors.AbstractExecutor{
					ExecutorOptions: options,
				},
			},
		}
	}

	common.RegisterExecutor("docker-ssh", common.ExecutorFactory{
		Create: create,
		Features: common.FeaturesInfo{
			Variables: true,
			Image:     true,
			Services:  true,
		},
	})
}
