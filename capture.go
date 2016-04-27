package main

import (
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
)

type tcpCapture struct {
	device   string
	closed   chan bool
	packages chan<- *NetPackage
	handle   *pcap.Handle
}

func StartTcpCapture(device string, target chan<- *NetPackage) (*tcpCapture, error) {
	handle, err := pcap.OpenLive(device, 128, true, 500)
	if err != nil {
		return nil, err
	}

	capture := &tcpCapture{
		device:   device,
		handle:   handle,
		closed:   make(chan bool, 0),
		packages: target,
	}

	go capture.run()
	return capture, nil
}

func (tc *tcpCapture) Close() error {
	tc.handle.Close()

	log.Info("Waiting for capture loop to terminate")
	<-tc.closed
	return nil
}

func (tc *tcpCapture) run() {
	defer close(tc.closed)

	// create a parser for a simple network stack
	var ipv4 layers.IPv4
	var eth layers.Ethernet
	var tcp layers.TCP
	parser := gopacket.NewDecodingLayerParser(layers.LayerTypeEthernet, &eth, &ipv4, &tcp)

	// make network packages available as a channel
	packetSource := gopacket.NewPacketSource(tc.handle, tc.handle.LinkType())

	var decoded []gopacket.LayerType
	for pkt := range packetSource.Packets() {
		parser.DecodeLayers(pkt.Data(), &decoded)

		netPackage := &NetPackage{Packages: 1}
		for _, layerType := range decoded {
			switch layerType {
			case layers.LayerTypeTCP:
				packageLength := len(tcp.Payload)
				if packageLength > 0 {
					netPackage.Timestamp = time.Now().UnixNano() / int64(time.Millisecond)
					netPackage.Len = uint16(packageLength)
					netPackage.Source.IP = ipv4.SrcIP.String()
					netPackage.Source.Port = uint16(tcp.SrcPort)
					netPackage.Destination.IP = ipv4.DstIP.String()
					netPackage.Destination.Port = uint16(tcp.DstPort)
				}
			}
		}

		if netPackage.Len > 0 {
			// send non-blocking, drop packages if consumer queue is full.
			select {
			case tc.packages <- netPackage:
			default:
			}
		}
	}
}
