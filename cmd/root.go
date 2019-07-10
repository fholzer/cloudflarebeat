package cmd

import (
	"github.com/fholzer/cloudflarebeat/beater"

	cmd "github.com/elastic/beats/libbeat/cmd"
)

// Name of this beat
var Name = "cloudflarebeat"

// RootCmd to handle beats cli
var RootCmd = cmd.GenRootCmd(Name, appVersion, beater.New)
