package main

import (
	"os"
	"path"

	"github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
	"github.com/bssthu/gitlab-ci-multi-runner/common"
	"github.com/bssthu/gitlab-ci-multi-runner/helpers/cli"
	"github.com/bssthu/gitlab-ci-multi-runner/helpers/formatter"

	_ "github.com/bssthu/gitlab-ci-multi-runner/commands"
	_ "github.com/bssthu/gitlab-ci-multi-runner/commands/helpers"
	_ "github.com/bssthu/gitlab-ci-multi-runner/executors/docker"
	_ "github.com/bssthu/gitlab-ci-multi-runner/executors/docker/machine"
	_ "github.com/bssthu/gitlab-ci-multi-runner/executors/parallels"
	_ "github.com/bssthu/gitlab-ci-multi-runner/executors/shell"
	_ "github.com/bssthu/gitlab-ci-multi-runner/executors/ssh"
	_ "github.com/bssthu/gitlab-ci-multi-runner/executors/virtualbox"
	_ "github.com/bssthu/gitlab-ci-multi-runner/shells"
)

func main() {
	defer func() {
		if r := recover(); r != nil {
			// log panics forces exit
			if _, ok := r.(*logrus.Entry); ok {
				os.Exit(1)
			}
			panic(r)
		}
	}()

	formatter.SetRunnerFormatter()

	app := cli.NewApp()
	app.Name = path.Base(os.Args[0])
	app.Usage = "a GitLab Runner"
	app.Version = common.AppVersion.ShortLine()
	cli.VersionPrinter = common.AppVersion.Printer
	app.Authors = []cli.Author{
		cli.Author{
			Name:  "Kamil Trzciński",
			Email: "ayufan@ayufan.eu",
		},
	}
	cli_helpers.LogRuntimePlatform(app)
	cli_helpers.SetupLogLevelOptions(app)
	cli_helpers.SetupCPUProfile(app)
	cli_helpers.FixHOME(app)
	app.Commands = common.GetCommands()
	app.CommandNotFound = func(context *cli.Context, command string) {
		logrus.Fatalln("Command", command, "not found.")
	}

	if err := app.Run(os.Args); err != nil {
		logrus.Fatal(err)
	}
}
