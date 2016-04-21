package main

import (
	"bytes"
	"fmt"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	"os"
	"os/signal"
	"regexp"
	"sync"
	"time"
)

var (
	sourceDestRe = regexp.MustCompile("TCP (.*):([0-9]{1,5}) > (.*):([0-9]{1,5}) .*")
	wg           sync.WaitGroup
)

type NetPackages struct {
	id        string
	timestamp int64
	eventType string
	version   string
	packages  []NetPackage
}

type NetPackage struct {
	source          string
	sourcePort      uint16
	destination     string
	destinationPort uint16
	timestamp       int64
	length          uint16
	packages        int
}

func (n *NetPackage) String() string {
	var b bytes.Buffer
	fmt.Fprintf(&b, "%s:%d -> %s:%d    %d bytes", n.source, n.sourcePort, n.destination, n.destinationPort, n.length)
	return b.String()
}

func shutDownHook() {
	go func() {
		signals := make(chan os.Signal, 1)
		signal.Notify(signals, os.Interrupt, os.Kill)
		<-signals
		fmt.Println("Done.")
		wg.Done()
	}()
}

func main() {
	devices, _ := pcap.FindAllDevs()
	wg.Add(1)
	shutDownHook()

	var ipv4 layers.IPv4
	var ipv6 layers.IPv6
	var eth layers.Ethernet
	var tcp layers.TCP

	parser := gopacket.NewDecodingLayerParser(layers.LayerTypeEthernet, &eth, &ipv4, &ipv6, &tcp)
	decoded := []gopacket.LayerType{}

	//fmt.Printf("%q\n", devices)
	for _, device := range devices {
		if device.Name == "en0" {
			fmt.Printf("Trying interface %s\n", device.Name)

			go func(deviceName string) {
				h, _ := pcap.OpenLive(deviceName, 65535, true, 500)
				defer h.Close()
				fmt.Printf("Listening to %s\n", deviceName)
				packetSource := gopacket.NewPacketSource(h, h.LinkType())
				for pkt := range packetSource.Packets() {
					parser.DecodeLayers(pkt.Data(), &decoded)
					if len(pkt.Data()) > 0 {
						//fmt.Printf("%s %s\n", device, pkt.String())
					}
					netPacket := &NetPackage{}
					for _, layerType := range decoded {
						switch layerType {
						//case layers.LayerTypeIPv6:
						//	fmt.Println("    IP6 ", ipv6.SrcIP, ipv6.DstIP)
						case layers.LayerTypeIPv4:
							//fmt.Printf("    IP4  %#v\n", ipv4)
							netPacket = &NetPackage{
								source:      ipv4.SrcIP.String(),
								destination: ipv4.DstIP.String(),
								length:      ipv4.Length,
								timestamp:   time.Now().Unix(),
							}
						/*case layers.LayerTypeEthernet:
						fmt.Println("    Ethernet ", eth)*/
						case layers.LayerTypeTCP:
							netPacket.sourcePort = uint16(tcp.SrcPort)
							netPacket.destinationPort = uint16(tcp.DstPort)
						}
					}
					if netPacket.length > 0 {
						fmt.Printf("********************* %s %s\n", device, netPacket.String())
					}

				}
			}(device.Name)
		}
	}
	wg.Wait()
}
