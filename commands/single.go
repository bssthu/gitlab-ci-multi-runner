package commands

import (
	"github.com/codegangsta/cli"
	"os"
	"time"

	log "github.com/Sirupsen/logrus"

	"gitlab.com/gitlab-org/gitlab-ci-multi-runner/common"
	"gitlab.com/gitlab-org/gitlab-ci-multi-runner/helpers"
	"net/http"
	"os/signal"
	"syscall"
)

func serverHelloWorld(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte("{}"))
}

func runServer(addr string) error {
	if len(addr) == 0 {
		return nil
	}

	http.HandleFunc("/", serverHelloWorld)
	err := http.ListenAndServe(addr, nil)
	if err != nil {
		log.Fatal(err)
		return err
	}

	return nil
}

func runHerokuURL(addr string) error {
	if len(addr) == 0 {
		return nil
	}

	for {
		resp, err := http.Get(addr)
		if err == nil {
			log.Infoln("HEROKU_URL acked!")
			defer resp.Body.Close()
		} else {
			log.Infoln("HEROKU_URL error: ", err)
		}
		time.Sleep(5 * time.Minute)
	}
}

func runSingle(c *cli.Context) {
	buildsDir := c.String("builds-dir")
	shell := c.String("shell")
	updateInterval := helpers.IntFromStringOrDefault(c.String("update-interval"), common.DefaultUpdateInterval)
	maxTraceOutputSize := helpers.IntFromStringOrDefault(c.String("max-trace-output-size"), common.DefaultMaxTraceOutputSize)
	runner := common.RunnerConfig{
		URL:       c.String("url"),
		Token:     c.String("token"),
		Executor:  c.String("executor"),
		BuildsDir: &buildsDir,
		Shell:     &shell,
		UpdateInterval: &updateInterval,
		MaxTraceOutputSize: &maxTraceOutputSize,
	}

	if len(runner.URL) == 0 {
		log.Fatalln("Missing URL")
	}
	if len(runner.Token) == 0 {
		log.Fatalln("Missing Token")
	}
	if len(runner.Executor) == 0 {
		log.Fatalln("Missing Executor")
	}

	checkInterval := time.Duration(helpers.IntFromStringOrDefault(c.String("check-interval"), common.DefaultCheckInterval))

	go runServer(c.String("addr"))
	go runHerokuURL(c.String("heroku-url"))

	signals := make(chan os.Signal)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)

	log.Println("Starting runner for", runner.URL, "with token", runner.ShortDescription(), "...")

	finished := false
	abortSignal := make(chan os.Signal)
	doneSignal := make(chan int, 1)

	go func() {
		interrupt := <-signals
		log.Warningln("Requested exit:", interrupt)
		finished = true

		go func() {
			for {
				abortSignal <- interrupt
			}
		}()

		select {
		case newSignal := <-signals:
			log.Fatalln("forced exit:", newSignal)
		case <-time.After(common.ShutdownTimeout * time.Second):
			log.Fatalln("shutdown timedout")
		case <-doneSignal:
		}
	}()

	for !finished {
		buildData, healthy := common.GetBuild(runner)
		if !healthy {
			log.Println("Runner is not healthy!")
			select {
			case <-time.After(common.NotHealthyCheckInterval * time.Second):
			case <-abortSignal:
			}
			continue
		}

		if buildData == nil {
			select {
			case <-time.After(checkInterval * time.Second):
			case <-abortSignal:
			}
			continue
		}

		newBuild := common.Build{
			GetBuildResponse: *buildData,
			Runner:           &runner,
			BuildAbort:       abortSignal,
		}
		newBuild.AssignID()
		newBuild.Run()
	}

	doneSignal <- 0
}

func init() {
	common.RegisterCommand(cli.Command{
		Name:   "run-single",
		Usage:  "start single runner",
		Action: runSingle,
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:   "token",
				Value:  "",
				Usage:  "Runner token",
				EnvVar: "RUNNER_TOKEN",
			},
			cli.StringFlag{
				Name:   "url",
				Value:  "",
				Usage:  "Runner URL",
				EnvVar: "CI_SERVER_URL",
			},
			cli.StringFlag{
				Name:   "executor",
				Value:  "shell",
				Usage:  "Executor",
				EnvVar: "RUNNER_EXECUTOR",
			},
			cli.StringFlag{
				Name:   "shell",
				Value:  common.GetDefaultShell(),
				Usage:  "Shell to use for run the script",
				EnvVar: "RUNNER_SHELL",
			},
			cli.StringFlag{
				Name:   "addr",
				Value:  "",
				Usage:  "Hello World Server",
				EnvVar: "",
			},
			cli.StringFlag{
				Name:   "heroku-url",
				Value:  "",
				Usage:  "Current application address",
				EnvVar: "HEROKU_URL",
			},
			cli.StringFlag{
				Name:   "builds-dir",
				Value:  "",
				Usage:  "Custom builds directory",
				EnvVar: "RUNNER_BUILDS_DIR",
			},
			cli.StringFlag{
				Name:   "check-interval",
				Value:  "",
				Usage:  "Interval between CI checks",
				EnvVar: "RUNNER_CHECK_INTERVAL",
			},
			cli.StringFlag{
				Name:   "update-interval",
				Value:  "",
				Usage:  "Interval between CI updates",
				EnvVar: "RUNNER_UPDATE_INTERVAL",
			},
			cli.StringFlag{
				Name:   "max-trace-output-size",
				Value:  "",
				Usage:  "Maximum size of the output trace",
				EnvVar: "RUNNER_MAX_TRACE_OUTPUT_SIZE",
			},
		},
	})
}
