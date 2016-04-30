package main

import (
	"strings"
	"sync"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/fsouza/go-dockerclient"
)

type dockerNameProvider struct {
	docker  *docker.Client
	mapping map[string]string
	ticker  *time.Ticker
	mutex   sync.RWMutex
}

func NewDockerNameProvider(client *docker.Client) NameProvider {
	provider := &dockerNameProvider{
		docker:  client,
		mapping: make(map[string]string),
		ticker:  time.NewTicker(2 * time.Second),
	}

	// update first time now
	provider.update()

	go provider.run()

	return provider
}

func (provider *dockerNameProvider) GetName(host string, port uint16) string {
	provider.mutex.RLock()
	defer provider.mutex.RUnlock()

	return provider.mapping[host]
}

func (provider *dockerNameProvider) update() {
	opts := docker.ListContainersOptions{}
	if containers, err := provider.docker.ListContainers(opts); err != nil {
		log.WithError(err).Warn("Could not enumerate docker containers")

	} else {
		mapping := make(map[string]string, len(containers))
		for _, container := range containers {
			if len(container.Names) == 0 {
				continue
			}

			container, err := provider.docker.InspectContainer(container.ID)
			if err != nil {
				log.WithError(err).WithField("id", container.ID).Warn("Could not inspect container")
				continue
			}

			if container.NetworkSettings != nil {
				for _, network := range container.NetworkSettings.Networks {
					mapping[network.IPAddress] = strings.Trim(container.Name, "/")
				}
			}
		}

		provider.mutex.Lock()
		defer provider.mutex.Unlock()

		provider.mapping = mapping
	}
}

func (provider *dockerNameProvider) run() {
	defer provider.ticker.Stop()

	for range provider.ticker.C {
		provider.update()
	}
}
