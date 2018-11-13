package a2n

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"sync"
	"time"
)

const MaxRemoteConfigBodySize = 1048576
const MinRemoteConfigSyncDelay = 60 * time.Second

type ConfigManager struct {
	mutex sync.RWMutex

	updateSignal chan struct{}
	remotePath   string
	nameToIP     map[string]net.IP
}

func NewConfigManager(remotePath string) *ConfigManager {
	return &ConfigManager{
		updateSignal: make(chan struct{}, 1),
		remotePath:   remotePath,
		nameToIP:     make(map[string]net.IP),
	}
}

func (m *ConfigManager) Start() {
	go m.Run()
}

func (m *ConfigManager) Run() {
	if m.remotePath == "" {
		return
	}

	for {
		m.silentlyUpdateRoutingTable()
		<-time.After(MinRemoteConfigSyncDelay)
		<-m.updateSignal
	}
}

func (m *ConfigManager) SignalUpdate() {
	select {
	case m.updateSignal <- struct{}{}:
	default:
	}
}

func (m *ConfigManager) GetNameByIP(name string) (net.IP, bool) {
	m.mutex.RLock()
	ret, ok := m.nameToIP[name]
	m.mutex.RUnlock()
	return ret, ok
}

func (m *ConfigManager) silentlyUpdateRoutingTable() {
	raw, err := m.fetch("routing_table")
	if err != nil {
		log.Printf("unable to fetch routing table: %+v", err)
		return
	}

	var table map[string]string
	err = json.Unmarshal(raw, &table)
	if err != nil {
		log.Printf("unable to parse routing table: %+v", err)
		return
	}

	out := make(map[string]net.IP)
	for k, v := range table {
		ip := net.ParseIP(v)
		if ip == nil {
			log.Printf("unable to parse ip: %s (name: %s)", v, k)
			continue
		}

		out[k] = ip
	}

	m.mutex.Lock()
	m.nameToIP = out
	m.mutex.Unlock()
}

func (m *ConfigManager) fetch(subPath string) ([]byte, error) {
	resp, err := http.Get(m.remotePath + "/" + subPath)
	if err != nil {
		return nil, err
	}

	ret, err := ioutil.ReadAll(io.LimitReader(resp.Body, MaxRemoteConfigBodySize))
	resp.Body.Close()
	return ret, err
}
