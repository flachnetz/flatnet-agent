package discovery

import (
	"net"
	"strings"
	"math"
	"strconv"
	"errors"
)

type netRange struct {
	Net     net.IPNet
	MinPort uint16
	MaxPort uint16
}

type netRangeProvider struct {
	Range    netRange
	Provider NameProvider
}

type multiNameProvider struct {
	providers map[string]netRangeProvider
}

type MultiNameProviderConfig map[string]NameProvider

func NewMultiNameProvider(config MultiNameProviderConfig) (NameProvider, error) {
	providers := make(map[string]netRangeProvider)
	for net, provider := range config {
		netRange, err := parseNetRange(net)
		if err != nil {
			return nil, err
		}

		providers[net] = netRangeProvider{*netRange, provider}
	}

	return &multiNameProvider{providers}, nil
}

func (mp *multiNameProvider) GetName(host string, port uint16) string {
	ip := net.ParseIP(host)

	if ip != nil {
		for _, entry := range mp.providers {
			if entry.Range.Contains(ip, port) {
				result := entry.Provider.GetName(host, port)
				if result != "" {
					return result
				}
			}
		}
	}

	return ""
}

func parseNetRange(input string) (*netRange, error) {
	parts := strings.Split(input, ":")
	partsCount := len(parts)

	if partsCount == 0 {
		_, net, err := net.ParseCIDR(input)
		if err != nil {
			return nil, err
		} else {
			return &netRange{*net, 0, math.MaxUint16}, nil
		}
	} else if partsCount == 1 {
		// parse first part as cidr
		_, net, err := net.ParseCIDR(parts[0])
		if err != nil {
			return nil, err
		}

		// then parse the port
		port, err := strconv.ParseUint(parts[1], 10, 16)
		if err != nil {
			return nil, err
		}

		return &netRange{*net, uint16(port), uint16(port)}, nil

	} else if partsCount == 2 {

		// parse first part as cidr
		_, net, err := net.ParseCIDR(parts[0])
		if err != nil {
			return nil, err
		}

		// then expect the (optional) minimum port
		var minPort, maxPort uint64 = 0, math.MaxUint16
		if parts[1] != "" {
			minPort, err = strconv.ParseUint(parts[1], 10, 16)
			if err != nil {
				return nil, err
			}
		}

		if parts[2] != "" {
			maxPort, err = strconv.ParseUint(parts[2], 10, 16)
			if err != nil {
				return nil, err
			}
		}

		if minPort > maxPort {
			// just swap. idiot user.
			minPort, maxPort = maxPort, minPort
		}

		return &netRange{*net, uint16(minPort), uint16(maxPort)}, nil
	}

	return nil, errors.New("Invalid format for cidr/port range. Must be like: 1.2.3.4/16:80:90")
}

func (nr *netRange) Contains(ip net.IP, port uint16) bool {
	return nr.MinPort <= port && port <= nr.MaxPort && nr.Net.Contains(ip)
}
