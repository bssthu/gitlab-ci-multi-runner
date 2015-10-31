package shell

import (
	"bytes"
	"errors"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/bssthu/gitlab-ci-multi-runner/common"
	"github.com/bssthu/gitlab-ci-multi-runner/executors"
	"github.com/bssthu/gitlab-ci-multi-runner/helpers"
)

type ShellExecutor struct {
	executors.AbstractExecutor
	cmd       *exec.Cmd
	scriptDir string
}

func (s *ShellExecutor) Prepare(globalConfig *common.Config, config *common.RunnerConfig, build *common.Build) error {
	if globalConfig != nil {
		s.Shell.User = globalConfig.User
	}

	err := s.AbstractExecutor.Prepare(globalConfig, config, build)
	if err != nil {
		return err
	}

	s.Println("Using Shell executor...")
	return nil
}

func (s *ShellExecutor) Start() error {
	s.Debugln("Starting shell command...")

	// Create execution command
	s.cmd = exec.Command(s.ShellScript.Command, s.ShellScript.Arguments...)
	if s.cmd == nil {
		return errors.New("Failed to generate execution command")
	}

	helpers.SetProcessGroup(s.cmd)

	// Fill process environment variables
	s.cmd.Env = append(os.Environ(), s.ShellScript.Environment...)
	s.cmd.Stdout = s.BuildLog
	s.cmd.Stderr = s.BuildLog

	if s.ShellScript.PassFile {
		scriptDir, err := ioutil.TempDir("", "build_script")
		if err != nil {
			return err
		}
		s.scriptDir = scriptDir

		scriptFile := filepath.Join(scriptDir, "script."+s.ShellScript.Extension)
		err = ioutil.WriteFile(scriptFile, s.ShellScript.GetScriptBytes(), 0700)
		if err != nil {
			return err
		}

		s.cmd.Args = append(s.cmd.Args, scriptFile)
	} else {
		s.cmd.Stdin = bytes.NewReader(s.ShellScript.GetScriptBytes())
	}

	// Start process
	err := s.cmd.Start()
	if err != nil {
		return errors.New("Failed to start process")
	}

	// Wait for process to exit
	go func() {
		s.BuildFinish <- s.cmd.Wait()
	}()
	return nil
}

func (s *ShellExecutor) Cleanup() {
	helpers.KillProcessGroup(s.cmd)

	if s.scriptDir != "" {
		os.RemoveAll(s.scriptDir)
	}

	s.AbstractExecutor.Cleanup()
}

func init() {
	options := executors.ExecutorOptions{
		DefaultBuildsDir: "builds",
		SharedBuildsDir:  true,
		Shell: common.ShellScriptInfo{
			Shell: common.GetDefaultShell(),
			Type:  common.LoginShell,
		},
		ShowHostname: false,
	}

	create := func() common.Executor {
		return &ShellExecutor{
			AbstractExecutor: executors.AbstractExecutor{
				ExecutorOptions: options,
			},
		}
	}

	common.RegisterExecutor("shell", common.ExecutorFactory{
		Create: create,
		Features: common.FeaturesInfo{
			Variables: true,
		},
	})
}
