// Put documentation here
package main

import (
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
	consulapi "github.com/hashicorp/consul/api"
)

type NameProvider interface {
	// Returns a name for the service or an empty string,
	// if no name could be determined.
	GetName(host string, port uint16) string
}

type NoopNameProvider struct{}

func (*NoopNameProvider) GetName(host string, port uint16) string {
	return ""
}

// Short description
type consulAddressKey struct {
	Ip   string
	Port uint16
}

type consulNameProvider struct {
	client   *consulapi.Client
	services map[consulAddressKey]string
	mutex    sync.RWMutex
	ticker   *time.Ticker
}

func NewConsulNameProvider(consul string) (*consulNameProvider, error) {
	config := consulapi.DefaultConfig()
	config.Address = consul
	client, err := consulapi.NewClient(config)
	if err != nil {
		return nil, err
	}

	services, err := consulListAllServices(client)
	if err != nil {
		return nil, err
	}

	provider := &consulNameProvider{
		client:   client,
		services: services,
		ticker:   time.NewTicker(10 * time.Second),
	}

	go provider.updateLoop()

	return provider, nil
}

func (cnp *consulNameProvider) Close() error {
	cnp.ticker.Stop()
	return nil
}

func (cnp *consulNameProvider) updateLoop() {
	for range cnp.ticker.C {
		logrus.Debug("Updating consul services now")
		services, err := consulListAllServices(cnp.client)
		if err != nil {
			logrus.WithError(err).Warn("Could not update consul services")
		}

		cnp.updateServices(services)
	}
}

func (cnp *consulNameProvider) updateServices(services map[consulAddressKey]string) {
	cnp.mutex.Lock()
	defer cnp.mutex.Unlock()

	cnp.services = services
}

func (cnp *consulNameProvider) GetName(host string, port uint16) string {
	cnp.mutex.RLock()
	defer cnp.mutex.RUnlock()

	key := consulAddressKey{host, port}
	return cnp.services[key]
}

// Short description
func consulListAllServices(consul *consulapi.Client) (map[consulAddressKey]string, error) {
	result := make(map[consulAddressKey]string)

	// get a list of all services
	catalog := consul.Catalog()
	services, _, err := catalog.Services(nil)
	if err != nil {
		return nil, err
	}

	// get info for every one of those services
	for serviceName, _ := range services {
		serviceInfos, _, err := catalog.Service(serviceName, "", nil)
		if err != nil {
			return nil, err
		}

		for _, service := range serviceInfos {
			result[consulAddressKey{service.Address, uint16(service.ServicePort)}] = service.ServiceName
			result[consulAddressKey{service.ServiceAddress, uint16(service.ServicePort)}] = service.ServiceName
		}
	}

	return result, nil
}
