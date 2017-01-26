package utils

import (
	"bytes"
	"os/exec"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/gravitational/trace"
)

// ExecCommand executes custom command, logs it's errors, returns output and error if any
func ExecCommand(command string, args ...string) (out []byte, err error) {
	cmd := exec.Command(command, args...)
	log.Debugf("executing '%s %s'", command, strings.Join(args, " "))
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	if err != nil {
		log.Error(err)
	}
	errtext := stderr.String()
	if errtext != "" {
		log.Error(errtext)
	}
	return stdout.Bytes(), trace.Wrap(err)
}
