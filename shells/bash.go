package shells

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/bssthu/gitlab-ci-multi-runner/common"
	"github.com/bssthu/gitlab-ci-multi-runner/helpers"
	"io"
	"path/filepath"
	"runtime"
	"strings"
	"text/template"
)

type BashShell struct {
	AbstractShell
}

func (b *BashShell) GetName() string {
	return "bash"
}

func (b *BashShell) echoColored(w io.Writer, text string) {
	coloredText := helpers.ANSI_BOLD_GREEN + text + helpers.ANSI_RESET
	io.WriteString(w, "echo " + helpers.ShellEscape(coloredText) + "\n")
}

func (b *BashShell) echoColoredFormat(w io.Writer, format string, a ...interface{}) {
	b.echoColored(w, fmt.Sprintf(format, a...))
}

func (b *BashShell) installGit(w io.Writer) error {
	installGitTmpl := `
function installGit() {
	PREFIX=
	if [[ $(id -u) -ne 0 ]]; then
		if which sudo >/dev/null 2>/dev/null; then
			PREFIX="sudo "
		elif [[ -x /usr/bin/sudo ]]; then
			PREFIX="/usr/bin/sudo "
		else
			echo "{{.ANSI_YELLOW}}WARNING: Cannot install git: missing sudo.{{.ANSI_RESET}}"
			return 1
		fi
	fi

	UPDATE_CMD=
	INSTALL_CMD=

	echo "Installing git..."
	if which apt-get >/dev/null 2>/dev/null; then # Debian/Ubuntu support
		UPDATE_CMD="apt-get update -y -q"
		INSTALL_CMD="apt-get install -y git-core ca-certificates"
	elif [[ -x /usr/bin/apt-get ]]; then
		UPDATE_CMD="/usr/bin/apt-get update -y -q"
		INSTALL_CMD="/usr/bin/apt-get install -y git-core ca-certificates"
	elif which yum >/dev/null 2>/dev/null; then # RedHat/CentOS support
		INSTALL_CMD="yum install -y git"
	elif [[ -x /usr/bin/yum ]]; then # RedHat/CentOS support
		INSTALL_CMD="/usr/bin/yum install -y git"
	else
		echo "{{.ANSI_YELLOW}}WARNING: Cannot install git: unsupported OS.{{.ANSI_RESET}}"
		return 1
	fi

	if [[ -n "$UPDATE_CMD" ]]; then
		echo "{{.ANSI_GREEN}$$ $PREFIX$UPDATE_CMD{{.ANSI_RESET}}"
		$PREFIX $UPDATE_CMD || true
	fi

	if [[ -n "$INSTALL_CMD" ]]; then
		echo "{{.ANSI_GREEN}$$ $PREFIX$INSTALL_CMD{{.ANSI_RESET}}"
		if ! $PREFIX $INSTALL_CMD; then
			echo "{{.ANSI_YELLOW}}WARNING: Cannot install git: the build may fail.{{.ANSI_RESET}}"
			return 1
		fi
	fi
}

if ! which git >/dev/null 2>/dev/null && ! [[ -x /usr/bin/git ]]; then
	installGit || true
fi
`
	template, err := template.New("").Parse(installGitTmpl)
	if err != nil {
		return err
	}

	var to = &struct {
		ANSI_GREEN string
		ANSI_YELLOW string
		ANSI_RESET string
	}{
		helpers.ANSI_BOLD_GREEN,
		helpers.ANSI_BOLD_YELLOW,
		helpers.ANSI_RESET,
	}

	err = template.Execute(w, to)
	if err != nil {
		return err
	}
	return nil
}

func (b *BashShell) writeCloneCmd(w io.Writer, build *common.Build, projectDir string) {
	b.echoColoredFormat(w, "Cloning repository...")
	io.WriteString(w, fmt.Sprintf("rm -rf %s\n", projectDir))
	io.WriteString(w, fmt.Sprintf("mkdir -p %s\n", projectDir))
	io.WriteString(w, fmt.Sprintf("git clone %s %s\n", helpers.ShellEscape(build.RepoURL), projectDir))
	io.WriteString(w, fmt.Sprintf("cd %s\n", projectDir))
}

func (b *BashShell) writeFetchCmd(w io.Writer, build *common.Build, projectDir string, gitDir string) {
	io.WriteString(w, fmt.Sprintf("if [[ -d %s ]]; then\n", gitDir))
	b.echoColoredFormat(w, "Fetching changes...")
	io.WriteString(w, fmt.Sprintf("cd %s\n", projectDir))
	io.WriteString(w, fmt.Sprintf("git clean -ffdx\n"))
	io.WriteString(w, fmt.Sprintf("git reset --hard > /dev/null\n"))
	io.WriteString(w, fmt.Sprintf("git remote set-url origin %s\n", helpers.ShellEscape(build.RepoURL)))
	io.WriteString(w, fmt.Sprintf("git fetch origin --tags -p\n"))
	io.WriteString(w, fmt.Sprintf("else\n"))
	b.writeCloneCmd(w, build, projectDir)
	io.WriteString(w, fmt.Sprintf("fi\n"))
}

func (b *BashShell) writeCheckoutCmd(w io.Writer, build *common.Build) {
	b.echoColoredFormat(w, "Checking out %s as %s...", build.Sha[0:8], build.RefName)
	io.WriteString(w, fmt.Sprintf("git checkout -qf %s\n", build.Sha))
}

func (b *BashShell) GenerateScript(info common.ShellScriptInfo) (*common.ShellScript, error) {
	var buffer bytes.Buffer
	w := bufio.NewWriter(&buffer)

	build := info.Build
	projectDir := build.FullProjectDir()
	projectDir = helpers.ToSlash(projectDir)
	gitDir := filepath.Join(projectDir, ".git")

	if len(build.Hostname) != 0 {
		io.WriteString(w, fmt.Sprintf("echo Running on $(hostname) via %s...", helpers.ShellEscape(build.Hostname)))
	} else {
		io.WriteString(w, "echo Running on $(hostname)...\n")
	}
	io.WriteString(w, "\n")
	io.WriteString(w, "echo\n")
	io.WriteString(w, "\n")

	// Set env variables from build script
	for _, keyValue := range b.GetVariables(build, projectDir, info.Environment) {
		io.WriteString(w, "export " + helpers.ShellEscape(keyValue) + "\n")
	}
	io.WriteString(w, "\n")
	b.installGit(w)
	io.WriteString(w, "\n")
	io.WriteString(w, "set -eo pipefail\n")
	io.WriteString(w, "\n")

	if build.AllowGitFetch {
		b.writeFetchCmd(w, build, helpers.ShellEscape(projectDir), helpers.ShellEscape(gitDir))
	} else {
		b.writeCloneCmd(w, build, helpers.ShellEscape(projectDir))
	}

	b.writeCheckoutCmd(w, build)
	io.WriteString(w, "\n")
	io.WriteString(w, "echo\n")
	io.WriteString(w, "\n")

	commands := build.Commands
	commands = strings.TrimSpace(commands)
	for _, command := range strings.Split(commands, "\n") {
		command = strings.TrimSpace(command)
		if !helpers.BoolOrDefault(build.Runner.DisableVerbose, false) {
			if command != "" {
				b.echoColored(w, "$ " + command)
			} else {
				io.WriteString(w, "echo\n")
			}
		}
		io.WriteString(w, command+"\n")
	}

	io.WriteString(w, "\n")

	w.Flush()

	// evaluate script in subcontext, this is required to close stdin
	scriptCommand := "#!/usr/bin/env bash\n: | eval " + helpers.ShellEscape(buffer.String())

	script := common.ShellScript{
		Script:      scriptCommand,
		Environment: b.GetVariables(build, projectDir, info.Environment),
	}

	// su
	if info.User != nil {
		script.Command = "su"
		if info.Type == common.LoginShell {
			script.Arguments = []string{"--shell", "/bin/bash", "--login", *info.User}
		} else {
			script.Arguments = []string{"--shell", "/bin/bash", *info.User}
		}
	} else {
		script.Command = "bash"
		if info.Type == common.LoginShell {
			script.Arguments = []string{"--login"}
		}
	}

	return &script, nil
}

func (b *BashShell) IsDefault() bool {
	return runtime.GOOS != "windows"
}

func init() {
	common.RegisterShell(&BashShell{})
}
