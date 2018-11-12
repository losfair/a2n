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

	nameToIP             map[string]net.IP
	allowedTargetsParsed []*net.IPNet
	allowArbitraryTarget bool
	backendHTTPS         bool
}

type RouterConfigTemplate struct {
	ListenAddr           string // for external use
	NameToIP             map[string]net.IP
	AllowedTargets       []string
	AllowArbitraryTarget bool
	BackendHTTPS         bool
}

func NewRouterConfig(tpl *RouterConfigTemplate) (*RouterConfig, error) {
	c := new(RouterConfig)
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

	c.nameToIP = tpl.NameToIP
	if c.nameToIP == nil {
		c.nameToIP = make(map[string]net.IP)
	}

	c.allowedTargetsParsed = allowedTargetsParsed
	c.allowArbitraryTarget = tpl.AllowArbitraryTarget
	c.backendHTTPS = tpl.BackendHTTPS

	c.mutex.Unlock()

	return nil
}

func BuildRouter(config *RouterConfig) http.Handler {
	return &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			config.mutex.RLock()
			defer config.mutex.RUnlock()

			if config.backendHTTPS {
				req.URL.Scheme = "https"
			} else {
				req.URL.Scheme = "http"
			}

			req.URL.Host = req.Host

			targetName := strings.Split(req.Host, ".")[0]
			var targetIP net.IP

			if ip, ok := config.nameToIP[targetName]; ok {
				targetIP = ip
			} else {
				targetIP = net.ParseIP(strings.Join(strings.Split(targetName, "-"), "."))
			}

			if targetIP == nil {
				log.Print("invalid address")
				req.URL.Host = "" // FIXME: this is a hack to trigger a proxy error.
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
				req.URL.Host = "" // FIXME: this is a hack to trigger a proxy error.
				return
			}

			if _, ok := req.Header["User-Agent"]; !ok {
				// explicitly disable User-Agent so it's not set to default value
				req.Header.Set("User-Agent", "")
			}
		},
	}
}
