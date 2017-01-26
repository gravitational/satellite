package utils

import (
	"os"
	"strings"
	"github.com/gravitational/trace"
	log "github.com/Sirupsen/logrus"
)

func SetupLogging(level string) error {
	lvl := strings.ToLower(level)
	if lvl == "debug" {
		trace.EnableDebug()
	}
	sev, err := log.ParseLevel(lvl)
	if err != nil {
		return err
	}
	log.SetLevel(sev)

	log.SetFormatter(&log.TextFormatter{})
	log.SetOutput(os.Stderr)
	return nil
}