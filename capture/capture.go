package capture

import (
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	"io"
	"net"
)

type Port uint16

type Endpoint struct {
	Host net.IP
	Port Port
}

// Number of milliseconds since the epoch.
type Timestamp uint64

type Packet struct {
	Source    Endpoint
	Target    Endpoint
	Timestamp Timestamp
	Length    uint32
}

type Capture interface {
	io.Closer
}

type pcapCapture struct {
	device  string
	packets chan <- Packet
	handle  *pcap.Handle
	log     log.FieldLogger

	// This channel will be closed when the handle is closed and the
	// capturing loop finished.
	closed  chan struct{}
}

func StartCapture(device string, target chan <- Packet) (Capture, error) {
	handle, err := pcap.OpenLive(device, 128, true, 500)
	if err != nil {
		return nil, err
	}

	capture := &pcapCapture{
		device:  device,
		handle:  handle,
		closed:  make(chan struct{}),
		packets: target,
		log:     log.WithField("device", device),
	}

	// start capturing in background
	go capture.run()

	return capture, nil
}

func (pc *pcapCapture) Close() error {
	pc.handle.Close()

	pc.log.Debug("Waiting for capture loop to terminate")
	<-pc.closed

	return nil
}

func (pc *pcapCapture) run() {
	defer close(pc.closed)
	defer pc.handle.Close()

	// create a parser for a simple network stack
	var ipv4 layers.IPv4
	var eth layers.Ethernet
	var tcp layers.TCP
	parser := gopacket.NewDecodingLayerParser(layers.LayerTypeEthernet, &eth, &ipv4, &tcp)

	// make network packages available as a channel
	packetSource := gopacket.NewPacketSource(pc.handle, pc.handle.LinkType())

	// we do decoding our self.
	packetSource.Lazy = true

	var decoded []gopacket.LayerType
	for pkt := range packetSource.Packets() {
		parser.DecodeLayers(pkt.Data(), &decoded)

		for _, layerType := range decoded {
			if layerType == layers.LayerTypeTCP && len(tcp.Payload) > 0 {
				pc.publishNonBlocking(Packet{
					Source:    Endpoint{ipv4.SrcIP, Port(tcp.SrcPort)},
					Target:    Endpoint{ipv4.DstIP, Port(tcp.DstPort)},
					Timestamp: Timestamp(time.Now().UnixNano() / int64(time.Millisecond)),
					Length:    uint32(len(tcp.Payload)),
				})
			}
		}
	}
}

func (pc *pcapCapture) publishNonBlocking(packet Packet) {
	select {
	case pc.packets <- packet:
	default:
		pc.log.Warn("Channel is full, dropping packet")
	}
}
