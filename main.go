// Package main implements truenas-exporter, which reports what a TrueNAS says about itself as a Prometheus metrics page.
package main

import (
	"os"

	"github.com/gookit/color"
	"github.com/katbyte/go-kt/clog"
	"github.com/katbyte/truenas-exporter/cli"
)

func main() {
	// the log level comes from TRUENAS_EXPORTER_LOG; read it once here, before anything logs
	clog.SetLevelFromEnv("TRUENAS_EXPORTER_LOG")

	cmd, err := cli.Make()
	if err != nil {
		clog.Log.Error(color.Sprintf("<red>truenas-exporter: building cmd</> %v", err))

		os.Exit(1)
	}

	if err := cmd.Execute(); err != nil {
		clog.Log.Error(color.Sprintf("<red>truenas-exporter:</> %v", err))

		os.Exit(1)
	}

	os.Exit(0)
}
