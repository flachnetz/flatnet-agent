package main

import (
	"fmt"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	"os"
	"os/signal"
	"sync"
	"time"
	"flag"
	"github.com/Shopify/sarama"
	"strings"
	"encoding/json"
	"net"
	"strconv"
	log "github.com/Sirupsen/logrus"
)

var (
	wg sync.WaitGroup
	brokers = flag.String("brokers", "docker:9092,192.168.59.100:9092", "The Kafka brokers to connect to, as a comma separated list")
	ignoredAddresses map[string][]uint16
	aggregatedPackages map[netAggregationKey]*NetPackage = make(map[netAggregationKey]*NetPackage)
	aggregatedPackagesChan chan *NetPackage = make(chan *NetPackage, 2048)
)

type netAggregationKey struct {
	source      Service
	destination Service
}

type NetPackages struct {
	Timestamp        int64
	ServicePackages  []NetPackage
	DurationInMillis int
	encoded          []byte `json: -`
}
type Service struct {
	Name string
	IP   string
	Port uint16
}

type NetPackage struct {
	Source      Service
	Destination Service
	Len         uint16
	Packages    int
	Timestamp   int64
}

func (n *NetPackages) Length() int {
	if n != nil && n.encoded == nil {
		// todo error handling
		n.encoded, _ = n.Encode()
	}
	return len(n.encoded)
}

func (n *NetPackages) Encode() ([]byte, error) {
	return json.Marshal(n)
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

func producerTicker() {
	durationInMillis := 2000
	flag.Parse()
	ticker := time.Tick(time.Duration(durationInMillis) * time.Millisecond)

	config := sarama.NewConfig()
	config.Producer.RequiredAcks = sarama.WaitForLocal       // Only wait for the leader to ack
	config.Producer.Compression = sarama.CompressionSnappy   // Compress messages
	config.Producer.Flush.Frequency = 500 * time.Millisecond // Flush batches every 500ms
	brokerList := strings.Split(*brokers, ",")

	// todo error handling
	producer, _ := sarama.NewAsyncProducer(brokerList, config)
	defer producer.Close()

	for {
		select {
		case now := <-ticker:
			aggregatedPackagesList := []NetPackage{}
			for _, p := range aggregatedPackages {
				p.Timestamp = now.Unix()
				aggregatedPackagesList = append(aggregatedPackagesList, *p)
			}
			aggregatedPackages = make(map[netAggregationKey]*NetPackage)

			netPackages := &NetPackages{
				Timestamp: now.Unix(),
				ServicePackages: aggregatedPackagesList,
				DurationInMillis: durationInMillis}
			if len(netPackages.ServicePackages) > 0 {
				log.Info(netPackages)
				producer.Input() <- &sarama.ProducerMessage{
					Topic: "flatnet_log",
					Key:   nil,
					Value: netPackages,
				}
			}
		case netPackage := <-aggregatedPackagesChan:
			if p, exists := aggregatedPackages[netAggregationKey{netPackage.Source, netPackage.Destination}]; exists == false {
				aggregatedPackages[netAggregationKey{netPackage.Source, netPackage.Destination}] = netPackage
			} else {
				p.Len = p.Len + netPackage.Len
				p.Packages += 1
			}
		}
	}

}

func main() {
	devices, _ := pcap.FindAllDevs()
	wg.Add(1)
	shutDownHook()

	ignoredAddresses = make(map[string][]uint16)
	ignoredAddresses["31.24.96.135"] = []uint16{0}
	ignoredAddresses["10.59.25.58"] = []uint16{9092}
	for _, host := range strings.Split(*brokers, ",") {
		hostPort := strings.Split(host, ":")

		addresses, _ := net.LookupHost(hostPort[0])
		for _, address := range addresses {
			port, _ := strconv.Atoi(hostPort[1])
			ignoredAddresses[address] = []uint16{uint16(port)}
		}
	}
	log.Print(ignoredAddresses)

	go producerTicker()

	//fmt.Printf("%q\n", devices)
	for _, device := range devices {
		if true {
			//device.Name == "en0" {
			fmt.Printf("Trying interface %s\n", device.Name)

			go func(deviceName string) {
				decoded := []gopacket.LayerType{}

				var ipv4 layers.IPv4
				var ipv6 layers.IPv6
				var eth layers.Ethernet
				var tcp layers.TCP
				parser := gopacket.NewDecodingLayerParser(layers.LayerTypeEthernet, &eth, &ipv4, &ipv6, &tcp)

				h, err := pcap.OpenLive(deviceName, 65535, true, 500)
				if err != nil {
					log.Error("Could not open ", device, ": ", err)
					return
				}
				defer h.Close()
				fmt.Printf("Listening to %s\n", deviceName)
				packetSource := gopacket.NewPacketSource(h, h.LinkType())
				for pkt := range packetSource.Packets() {
					parser.DecodeLayers(pkt.Data(), &decoded)
					if len(pkt.Data()) > 0 {
						//fmt.Printf("%s %s\n", device, pkt.String())
					}
					netPackage := &NetPackage{Source: Service{}, Destination: Service{}, Packages: 1}
					for _, layerType := range decoded {
						switch layerType {
						// case layers.LayerTypeIPv4:
						case layers.LayerTypeTCP:
							netPackage.Source.IP = ipv4.SrcIP.String()
							netPackage.Destination.IP = ipv4.DstIP.String()
							netPackage.Len = ipv4.Length
							netPackage.Timestamp = time.Now().Unix()
							netPackage.Source.Port = uint16(tcp.SrcPort)
							netPackage.Destination.Port = uint16(tcp.DstPort)
						}
					}
					if netPackage.Len > 0  && notIgnored(netPackage.Destination.IP, netPackage.Destination.Port) &&
						notIgnored(netPackage.Source.IP, netPackage.Source.Port) {
						aggregatedPackagesChan <- netPackage

					}

				}
			}(device.Name)
		}
	}
	wg.Wait()
}

func notIgnored(ip string, port uint16) bool {
	h := ignoredAddresses[ip]
	if h != nil && len(h) > 0 {
		//log.Printf("%s:%#v\n" , ip, h)
		for _, p := range h {
			if p == 0 || p == port {
				return false
			}
		}
	}
	return true
}
