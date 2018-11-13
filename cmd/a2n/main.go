package main

import (
	"encoding/json"
	"flag"
	"github.com/losfair/a2n"
	"io/ioutil"
	"log"
	"net/http"
)

type Config struct {
	RemoteConfigPath string
	Control          string
	Routers          []*a2n.RouterConfigTemplate
}

var servers = make(map[string]*a2n.RouterConfig)
var configFile string

func buildControlMux() http.Handler {
	mux := http.NewServeMux()

	// Reloads all router configurations.
	// No changes will be applied except Routers.
	// This only updates existing routers. New routers won't be added, and outdated routers won't be deleted.
	mux.HandleFunc("/reload", func(w http.ResponseWriter, r *http.Request) {
		configContent, err := ioutil.ReadFile(configFile)
		if err != nil {
			log.Printf("unable to read config file '%s': %+v", configFile, err)
			return
		}

		var config Config
		err = json.Unmarshal(configContent, &config)
		if err != nil {
			log.Printf("unable to parse config file '%s': %+v", configFile, err)
			return
		}

		for _, r := range config.Routers {
			if rc, ok := servers[r.ListenAddr]; ok {
				err := rc.Update(r)
				if err != nil {
					log.Printf("unable to update configuration of router '%s': %+v", r.ListenAddr, err)
				} else {
					log.Printf("configuration of router '%s' updated", r.ListenAddr)
				}
			}
		}
	})

	return mux
}

func main() {
	flag.Parse()

	_configFile := flag.String("config", "config.json", "path to the configuration file")
	configFile = *_configFile
	configContent, err := ioutil.ReadFile(configFile)
	if err != nil {
		log.Fatalf("unable to read config file '%s': %+v", configFile, err)
	}

	var config Config
	err = json.Unmarshal(configContent, &config)
	if err != nil {
		log.Fatalf("unable to parse config file '%s': %+v", configFile, err)
	}

	manager := a2n.NewConfigManager(config.RemoteConfigPath)
	manager.Start()

	for _, router := range config.Routers {
		router := router

		if _, ok := servers[router.ListenAddr]; ok {
			panic("duplicate listen addresses")
		}

		rc, err := a2n.NewRouterConfig(manager, router)
		if err != nil {
			log.Fatalf("unable to load router config (%s): %+v", router.ListenAddr, err)
		}

		server := &http.Server{
			Addr:    router.ListenAddr,
			Handler: a2n.BuildRouter(rc),
		}

		go func() {
			err := server.ListenAndServe()
			if err != nil {
				log.Printf("router on %s exited with error: %+v", router.ListenAddr, err)
			}
		}()

		servers[router.ListenAddr] = rc
		log.Printf("router on %s started", router.ListenAddr)
	}

	if len(config.Control) > 0 {
		mux := buildControlMux()
		server := &http.Server{
			Addr:    config.Control,
			Handler: mux,
		}
		go func() {
			err := server.ListenAndServe()
			if err != nil {
				log.Printf("control interface on %s exited with error: %+v", config.Control, err)
			}
		}()
		log.Printf("control interface on %s started", config.Control)
	}

	select {}
}
