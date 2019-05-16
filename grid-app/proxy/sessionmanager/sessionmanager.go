package sessionmanager

import (
	"os/exec"
)

type WorkspaceSession struct {
	GoCmd    *exec.Cmd
	NodeCmd  *exec.Cmd
	Port     int
	TermPort int
}
