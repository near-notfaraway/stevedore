package sd_config

import (
	"flag"
	"fmt"
)

type Action uint8

const (
	ActionRun Action = iota
	ActionTest
	ActionSignal
	ActionVersion
)

const CurrentVersion = "1.0.0"

type Option struct {
	Config  string // -c or --config opt
	Test    bool   // -t or --test opt
	Signal  string // -s or --signal opt
	Version bool   // -v or --version
}

// ParseOption registers supported command-line flags and parses the current arguments.
func ParseOption() *Option {
	flag.Usage = showUsage
	opt := Option{}

	flag.StringVar(&opt.Config, "c", "./stevedore.config.json", "Path to config file")
	flag.StringVar(&opt.Config, "config", "./stevedore.config.json", "Path to config file")

	flag.BoolVar(&opt.Test, "t", false, "Test config file")
	flag.BoolVar(&opt.Test, "test", false, "Test config file")

	flag.StringVar(&opt.Signal, "s", "", "Send signal to the process")
	flag.StringVar(&opt.Signal, "signal", "", "Send signal to the process")

	flag.BoolVar(&opt.Version, "v", false, "Show version")
	flag.BoolVar(&opt.Version, "version", false, "Show version")

	flag.Parse()
	return &opt
}

// Action validates mutually exclusive options and returns the requested operation.
func (o *Option) Action() (Action, error) {
	actionCount := 0
	if o.Version {
		actionCount++
	}
	if o.Test {
		actionCount++
	}
	if o.Signal != "" {
		actionCount++
	}
	if actionCount > 1 {
		return ActionRun, fmt.Errorf("version, test, and signal options are mutually exclusive")
	}
	if o.Version {
		return ActionVersion, nil
	}
	if o.Test {
		return ActionTest, nil
	}
	if o.Signal != "" {
		switch o.Signal {
		case "reload", "stop", "quit":
			return ActionSignal, nil
		default:
			return ActionRun, fmt.Errorf("signal %q is invalid", o.Signal)
		}
	}
	return ActionRun, nil
}

func showUsage() {
	fmt.Printf(`Usage:
	    stevedore [options...] [-htv] [-c <file>] [-s <signal>]

Options:
    -h/--help               Show help
    -c/--config <file>      Path to config file
    -t/--test               Test config file
    -s/--signal <sig>       Send signal to the process
                            The argument signal can be one of follows:
                              1) stop    SIGTERM
                              2) quit    SIGQUIT
                              3) reload  SIGHUP
    -v/--version            Show Version
`,
	)
}
