package docker

import (
	"bytes"
	"fmt"

	"github.com/fsouza/go-dockerclient"

	"github.com/bssthu/gitlab-ci-multi-runner/common"
	"github.com/bssthu/gitlab-ci-multi-runner/executors"
)

type DockerCommandExecutor struct {
	DockerExecutor
}

func (s *DockerCommandExecutor) Start() error {
	s.Debugln("Starting Docker command...")

	// Create container
	err := s.createBuildContainer(s.ShellScript.GetCommandWithArguments())
	if err != nil {
		return err
	}

	// Wait for process to exit
	go func() {
		attachContainerOptions := docker.AttachToContainerOptions{
			Container:    s.buildContainer.ID,
			InputStream:  bytes.NewBufferString(s.ShellScript.Script),
			OutputStream: s.BuildLog,
			ErrorStream:  s.BuildLog,
			Logs:         true,
			Stream:       true,
			Stdin:        true,
			Stdout:       true,
			Stderr:       true,
			RawTerminal:  false,
		}

		s.Debugln("Attaching to container...")
		err := s.client.AttachToContainer(attachContainerOptions)
		if err != nil {
			s.BuildFinish <- err
			return
		}

		s.Debugln("Waiting for container...")
		exitCode, err := s.client.WaitContainer(s.buildContainer.ID)
		if err != nil {
			s.BuildFinish <- err
			return
		}

		if exitCode == 0 {
			s.BuildFinish <- nil
		} else {
			s.BuildFinish <- fmt.Errorf("exit code %d", exitCode)
		}
	}()
	return nil
}

func init() {
	options := executors.ExecutorOptions{
		DefaultBuildsDir: "/builds",
		SharedBuildsDir:  false,
		Shell: common.ShellScriptInfo{
			Shell: "bash",
			Type:  common.NormalShell,
		},
		ShowHostname:     true,
		SupportedOptions: []string{"image", "services"},
	}

	create := func() common.Executor {
		return &DockerCommandExecutor{
			DockerExecutor: DockerExecutor{
				AbstractExecutor: executors.AbstractExecutor{
					ExecutorOptions: options,
				},
			},
		}
	}

	common.RegisterExecutor("docker", common.ExecutorFactory{
		Create: create,
		Features: common.FeaturesInfo{
			Variables: true,
			Image:     true,
			Services:  true,
		},
	})
}
