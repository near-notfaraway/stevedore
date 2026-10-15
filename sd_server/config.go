package sd_server

import (
	"fmt"
	"net"

	"github.com/near-notfaraway/stevedore/sd_config"
	"github.com/near-notfaraway/stevedore/sd_socket"
	"github.com/sirupsen/logrus"
)

// ValidateConfig checks the complete startup configuration without creating runtime resources.
func ValidateConfig(config *sd_config.Config) error {
	if config == nil {
		return fmt.Errorf("config is required")
	}
	requiredSections := []struct {
		name  string
		value any
	}{
		{name: "Common", value: config.Common},
		{name: "PProf", value: config.PProf},
		{name: "Log", value: config.Log},
		{name: "Server", value: config.Server},
		{name: "Upload", value: config.Upload},
		{name: "Session", value: config.Session},
	}
	for _, section := range requiredSections {
		if section.value == nil {
			return fmt.Errorf("%s config is required", section.name)
		}
	}

	if _, err := logrus.ParseLevel(config.Log.Level); err != nil {
		return fmt.Errorf("log level is invalid: %w", err)
	}
	if config.Log.Path == "" {
		return fmt.Errorf("log path is required")
	}
	if config.Log.MaxAgeHour <= 0 {
		return fmt.Errorf("log max age must be positive")
	}
	if config.Log.RotationTimeHour <= 0 {
		return fmt.Errorf("log rotation time must be positive")
	}
	if config.PProf.Open {
		if _, err := net.ResolveTCPAddr("tcp", config.PProf.ServerAddr); err != nil {
			return fmt.Errorf("pprof server address is invalid: %w", err)
		}
	}
	if sd_socket.ResolveUDPSockaddr(config.Server.ListenAddr) == nil {
		return fmt.Errorf("server listen address %q is invalid", config.Server.ListenAddr)
	}
	if config.Server.MaxTryTimes <= 0 {
		return fmt.Errorf("server max try times must be positive")
	}
	if config.Session.RecycleIntervalSec <= 0 {
		return fmt.Errorf("session recycle interval must be positive")
	}
	if config.Session.TimeoutSec < 0 {
		return fmt.Errorf("session timeout must not be negative")
	}
	if _, err := validateUploadConfig(config.Upload, false); err != nil {
		return fmt.Errorf("upload config is invalid: %w", err)
	}
	return nil
}
