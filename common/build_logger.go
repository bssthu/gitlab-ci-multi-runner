package common

import (
	"fmt"

	"github.com/Sirupsen/logrus"
	"github.com/bssthu/gitlab-ci-multi-runner/helpers"
)

type BuildLogger struct {
	log   BuildTrace
	entry *logrus.Entry
}

func (e *BuildLogger) sendLog(logger func(args ...interface{}), logPrefix string, args ...interface{}) {
	if e.log != nil {
		fmt.Fprint(e.log, logPrefix+fmt.Sprintln(args...)+helpers.ANSI_RESET)

		if e.log.IsStdout() {
			return
		}
	}

	if len(args) == 0 {
		return
	}

	logger(args...)
}

func (e *BuildLogger) Debugln(args ...interface{}) {
	if e.entry == nil {
		return
	}
	e.entry.Debugln(args...)
}

func (e *BuildLogger) Println(args ...interface{}) {
	if e.entry == nil {
		return
	}
	e.sendLog(e.entry.Debugln, helpers.ANSI_CLEAR, args...)
}

func (e *BuildLogger) Infoln(args ...interface{}) {
	if e.entry == nil {
		return
	}
	e.sendLog(e.entry.Println, helpers.ANSI_BOLD_GREEN, args...)
}

func (e *BuildLogger) Warningln(args ...interface{}) {
	if e.entry == nil {
		return
	}
	e.sendLog(e.entry.Warningln, helpers.ANSI_YELLOW+"WARNING: ", args...)
}

func (e *BuildLogger) SoftErrorln(args ...interface{}) {
	if e.entry == nil {
		return
	}
	e.sendLog(e.entry.Warningln, helpers.ANSI_BOLD_RED+"ERROR: ", args...)
}

func (e *BuildLogger) Errorln(args ...interface{}) {
	if e.entry == nil {
		return
	}
	e.sendLog(e.entry.Errorln, helpers.ANSI_BOLD_RED+"ERROR: ", args...)
}

func NewBuildLogger(log BuildTrace, entry *logrus.Entry) BuildLogger {
	return BuildLogger{
		log:   log,
		entry: entry,
	}
}
