package main

import (
	"flag"
	"fmt"
	"github.com/near-notfaraway/stevedore/sd_config"
	"github.com/near-notfaraway/stevedore/sd_server"
	"github.com/near-notfaraway/stevedore/sd_util"
)

func main() {
	// parse option config
	configPath := flag.String("config", "../etc/config.json", "config file path")
	flag.Parse()

	// parse file config
	var config sd_config.Config
	if err := sd_util.UnmarshalFile(*configPath, &config); err != nil {
		panic(fmt.Errorf("init config failed: %w", err))
	}

	// listen and serve
	panic(sd_server.NewServer(&config).ListenAndServe())
}
