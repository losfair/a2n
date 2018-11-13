package a2n

import (
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"strings"
	"sync"
)

type RouterConfig struct {
	mutex sync.RWMutex

	manager              *ConfigManager
	allowedTargetsParsed []*net.IPNet
	allowArbitraryTarget bool
	backendHTTPS         bool
}

type RouterConfigTemplate struct {
	ListenAddr           string // for external use
	AllowedTargets       []string
	AllowArbitraryTarget bool
	BackendHTTPS         bool
}

func NewRouterConfig(manager *ConfigManager, tpl *RouterConfigTemplate) (*RouterConfig, error) {
	c := new(RouterConfig)
	c.manager = manager
	if err := c.Update(tpl); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *RouterConfig) Update(tpl *RouterConfigTemplate) error {
	allowedTargetsParsed := make([]*net.IPNet, 0)
	for _, t := range tpl.AllowedTargets {
		_, n, err := net.ParseCIDR(t)
		if err != nil {
			return err
		}
		allowedTargetsParsed = append(allowedTargetsParsed, n)
	}

	// No error is allowed from here on.

	c.mutex.Lock()

	c.allowedTargetsParsed = allowedTargetsParsed
	c.allowArbitraryTarget = tpl.AllowArbitraryTarget
	c.backendHTTPS = tpl.BackendHTTPS

	c.mutex.Unlock()

	return nil
}

func BuildRouter(config *RouterConfig) http.Handler {
	return &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			config.manager.SignalUpdate()

			config.mutex.RLock()
			defer config.mutex.RUnlock()

			if config.backendHTTPS {
				req.URL.Scheme = "https"
			} else {
				req.URL.Scheme = "http"
			}

			targetName := strings.Split(req.Host, ".")[0]
			var targetIP net.IP

			if ip, ok := config.manager.GetNameByIP(targetName); ok {
				targetIP = ip
			} else {
				targetIP = net.ParseIP(strings.Join(strings.Split(targetName, "-"), "."))
			}

			if targetIP == nil {
				log.Print("invalid address")
				return
			}

			targetAllowed := false
			if config.allowArbitraryTarget {
				targetAllowed = true
			} else {
				for _, n := range config.allowedTargetsParsed {
					if n.Contains(targetIP) {
						targetAllowed = true
						break
					}
				}
			}

			if !targetAllowed {
				log.Print("access denied")
				return
			}

			if _, ok := req.Header["User-Agent"]; !ok {
				// explicitly disable User-Agent so it's not set to default value
				req.Header.Set("User-Agent", "")
			}

			req.Header.Set("Host", req.Host)
			req.URL.Host = targetIP.String() // req.URL.Host is left empty before here; therefore, any early returns trigger a proxy error.
		},
	}
}
