package sd_config

import (
	"flag"
	"fmt"
	"github.com/near-notfaraway/stevedore/sd_util"
	"os"
	"os/exec"
	"syscall"
)

const (
	CurrentVersion = "1.0.0"
	DaemonEnvKey   = "STEVEDORE_DAEMON"
	DaemonEnvValue = "True"
)

type Option struct {
	Config  string
	Daemon  bool
	Signal  string
	Version bool
}

func ParseOption() *Option {
	flag.Usage = showUsage
	opt := Option{}

	flag.BoolVar(&opt.Daemon, "d", false, "Running as a daemon")
	flag.BoolVar(&opt.Daemon, "daemon", false, "Running as a daemon")

	flag.StringVar(&opt.Config, "c", "./stevedore.config.json", "Path to config file")
	flag.StringVar(&opt.Config, "config", "./stevedore.config.json", "Path to config file")

	flag.StringVar(&opt.Signal, "s", "", "Send signal to the process")
	flag.StringVar(&opt.Signal, "signal", "", "Send signal to the process")

	flag.BoolVar(&opt.Version, "v", false, "Show version")
	flag.BoolVar(&opt.Version, "version", false, "Show version")

	flag.Parse()
	return &opt
}

func (o *Option) Handle() {
	o.handleVersionOpt()
	o.handleConfigOpt()
	o.handleSignalOpt()
	o.handleDaemonOpt()
}

func (o *Option) handleVersionOpt() {
	if o.Version {
		fmt.Printf("%s\n", CurrentVersion)
		os.Exit(0)
	}
}

func (o *Option) handleConfigOpt() {
	if o.Config != "" {
		if err := sd_util.UnmarshalFile(o.Config, GlobalConfig); err != nil {
			fmt.Printf("init config failed: %s", err)
			os.Exit(1)
		}
	}

	if err := GlobalConfig.TestAndComplete(); err != nil {
		fmt.Printf("test config failed: %s", err)
		os.Exit(1)
	}
}

func (o *Option) handleSignalOpt() {
	if o.Signal != "" {
		switch o.Signal {
		case "kill":
			pid, err := sd_util.GetPid(GlobalConfig.Common.PidPath)
			if err != nil {
				fmt.Printf("get pid failed: %s\n", err)
				os.Exit(1)
			}

			if pid == -1 {
				fmt.Printf("stevedore is not running\n")
				os.Exit(1)
			}

			if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
				fmt.Printf("send signal for kill failed: %s\n", err)
				os.Exit(1)
			}

			if err := sd_util.RemovePid(GlobalConfig.Common.PidPath); err != nil {
				fmt.Printf("remove pid failed: %s\n", err)
			}

			fmt.Printf("kill stevedore[%d] succeed\n", pid)
			os.Exit(0)

		case "stop":
			pid, err := sd_util.GetPid(GlobalConfig.Common.PidPath)
			if err != nil {
				fmt.Printf("get pid failed: %s\n", err)
				os.Exit(1)
			}

			if pid == -1 {
				fmt.Printf("stevedore is not running\n")
				os.Exit(1)
			}

			if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
				fmt.Printf("send signal for stop failed: %s\n", err)
				os.Exit(1)
			}

			if err := sd_util.RemovePid(GlobalConfig.Common.PidPath); err != nil {
				fmt.Printf("remove pid failed: %s\n", err)
			}

			fmt.Printf("stop stevedore[%d] succeed\n", pid)
			os.Exit(0)

		case "reload":
			pid, err := sd_util.GetPid(GlobalConfig.Common.PidPath)
			if err != nil {
				fmt.Printf("get pid failed: %s\n", err)
				os.Exit(1)
			}

			if pid == -1 {
				fmt.Printf("stevedore is not running\n")
				os.Exit(1)
			}

			if err := syscall.Kill(pid, syscall.SIGHUP); err != nil {
				fmt.Printf("send signal for reload failed: %s\n", err)
				os.Exit(1)
			}

			fmt.Printf("reload stevedore[%d] succeed\n", pid)
			os.Exit(0)

		default:
			fmt.Printf("signal is invalid: %s\n", o.Signal)
			os.Exit(1)
		}
	}
}

func (o *Option) handleDaemonOpt() {
	if o.Daemon {
		daemonEnv := os.Getenv(DaemonEnvKey)
		if daemonEnv == DaemonEnvValue {
			return
		}

		cmd := exec.Command(os.Args[0], os.Args[1:]...)
		cmd.Env = []string{fmt.Sprintf("%s=%s", DaemonEnvKey, DaemonEnvValue)}
		if err := cmd.Start(); err != nil {
			fmt.Printf("start stevedore daemon failed, error: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("start stevedore[%d] daemon\n", cmd.Process.Pid)
		os.Exit(0)
	}
}

func showUsage() {
	fmt.Printf(`Usage:
    stevedore [options...] [-c <file>] [-s <signal>]	

Options:
    -h/--help               Show help
    -c/--config <file>      Path to config file
    -d/--daemon             Running as a daemon
    -s/--signal <sig>       Send signal to the process
                            The argument signal can be one of follows:
                              1) kill    SIGKILL
                              2) stop    SIGTERM
                              3) reload  SIGHUP
    -v/--version            Show Version
`,
	)
}
