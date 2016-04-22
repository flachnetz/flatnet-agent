// Put documentation here
package main

import (
	"sync"
	"time"

	consulapi "github.com/hashicorp/consul/api"
)

// Short description
type Address struct {
	Ip   string
	Port int
}

// Short Description
var consulServices map[Address]string = make(map[Address]string)
var mutex sync.Mutex
var firstUpdate sync.Once

// Short description
func getServices(consul *consulapi.Client) map[string][]string {
	catalog := consul.Catalog()

	results, _, err := catalog.Services(nil)
	if err != nil {
		return nil
	}

	return results
}

// Short description
func getServiceInfo(consul *consulapi.Client, entry string, tag string) {
	catalog := consul.Catalog()
	serviceInfo, _, err := catalog.Service(entry, tag, nil)
	if err != nil {
		return
	}
	for _, service := range serviceInfo {
		var vservice Address
		var node Address
		node.Ip = service.Address
		node.Port = service.ServicePort
		vservice.Ip = service.ServiceAddress
		vservice.Port = service.ServicePort

		consulServices[node] = service.ServiceName
		consulServices[vservice] = service.ServiceName
	}
}

// Short description
func updateServices(consulAddress string) {
	config := consulapi.DefaultConfig()
	config.Address = consulAddress
	consul, err := consulapi.NewClient(config)
	if err != nil {
		return
	}

	var catalogEntries map[string][]string

	catalogEntries = getServices(consul)

	for catalogEntry, catalogTags := range catalogEntries {
		for _, tag := range catalogTags {
			getServiceInfo(consul, catalogEntry, tag)
		}
	}
}

func updateServicesLoop(consulAddress string) {
	tick := time.Tick(1 * time.Minute)
	for range tick {
		go func() {
			mutex.Lock()
			defer mutex.Unlock()
			updateServices(consulAddress)
		}()
	}
}

// Short description
func ServiceName(consulAddress string, ip string, port int) string {
	firstUpdate.Do(func() {
		mutex.Lock()
		defer mutex.Unlock()

		updateServices(consulAddress)

		go updateServicesLoop(consulAddress)
	})

	mutex.Lock()
	defer mutex.Unlock()

	node := Address{Ip: ip, Port: port}
	result := consulServices[node]

	return result
}