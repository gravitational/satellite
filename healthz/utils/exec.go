package utils

import (
	"bytes"
	"os/exec"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/gravitational/trace"
)

// ExecCommand executes custom command with kubectl
func ExecCommand(command string, args ...string) ([]byte, error) {
	cmd := exec.Command(command, args...)
	log.Debugf("Executing '%s %s'", command, strings.Join(args, " "))
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		log.Error(err)
	}
	errtext := stderr.String()
	if errtext != "" {
		log.Error(errtext)
	}
	return stdout.Bytes(), trace.Wrap(err)
}
