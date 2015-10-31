package main

import (
	"os"
	"path"

	log "github.com/Sirupsen/logrus"
	"github.com/bssthu/gitlab-ci-multi-runner/helpers/cli"
	"github.com/codegangsta/cli"

	"fmt"
	_ "github.com/bssthu/gitlab-ci-multi-runner/commands"
	"github.com/bssthu/gitlab-ci-multi-runner/common"
	_ "github.com/bssthu/gitlab-ci-multi-runner/shells"
	_ "github.com/bssthu/gitlab-ci-multi-runner/executors/docker"
	_ "github.com/bssthu/gitlab-ci-multi-runner/executors/parallels"
	_ "github.com/bssthu/gitlab-ci-multi-runner/executors/shell"
	_ "github.com/bssthu/gitlab-ci-multi-runner/executors/ssh"
)

var NAME = "gitlab-ci-multi-runner"
var VERSION = "dev"
var REVISION = "HEAD"

func init() {
	common.NAME = NAME
	common.VERSION = VERSION
	common.REVISION = REVISION
}

func main() {
	app := cli.NewApp()
	app.Name = path.Base(os.Args[0])
	app.Usage = "a GitLab Runner"
	app.Version = fmt.Sprintf("%s (%s)", common.VERSION, common.REVISION)
	app.Author = "Kamil Trzciński"
	app.Email = "ayufan@ayufan.eu"
	cli_helpers.SetupLogLevelOptions(app)
	app.Commands = common.GetCommands()

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
