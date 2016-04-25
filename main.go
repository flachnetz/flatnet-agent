package main

import (
	"flag"
	"os"
	"os/signal"
	"strings"
	"time"

	"io"

	log "github.com/Sirupsen/logrus"
	"github.com/VividCortex/godaemon"
	"github.com/google/gopacket/pcap"
)

type NetPackages struct {
	Timestamp        int64
	ServicePackages  []*NetPackage
	DurationInMillis int

	encoded []byte
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

func aggregate(packages <-chan *NetPackage, consumer chan<- *NetPackages, nameProvider NameProvider) {
	type aggregationKey struct {
		source      Service
		destination Service
	}

	// aggregate for a few seconds before sending a packet
	aggregationInterval := 2 * time.Second
	ticker := time.NewTicker(aggregationInterval)
	defer ticker.Stop()

	aggregated := make(map[aggregationKey]*NetPackage)
	for {
		select {
		case now := <-ticker.C:
			if len(aggregated) > 0 {
				aggregatedPackagesList := []*NetPackage{}
				for _, p := range aggregated {
					aggregatedPackagesList = append(aggregatedPackagesList, p)
				}

				// reset the aggregation map
				aggregated = make(map[aggregationKey]*NetPackage)

				result := &NetPackages{
					Timestamp:        now.UnixNano() / int64(time.Millisecond),
					ServicePackages:  aggregatedPackagesList,
					DurationInMillis: int(aggregationInterval / time.Millisecond),
				}

				// send non-blocking, drop packages if consumer queue is full.
				select {
				case consumer <- result:
				default:
				}
			}

		case netPackage, ok := <-packages:
			if !ok {
				// end of stream reached...
				return
			}

			key := aggregationKey{netPackage.Source, netPackage.Destination}

			if p, exists := aggregated[key]; exists {
				p.Len += netPackage.Len
				p.Packages += 1
			} else {
				aggregated[key] = netPackage

				netPackage.Source.Name = nameProvider.GetName(netPackage.Source.IP, netPackage.Source.Port)
				netPackage.Destination.Name = nameProvider.GetName(netPackage.Destination.IP, netPackage.Destination.Port)
			}
		}
	}
}

func main() {
	flagConsul := flag.String("consul", "", "Address of consul")
	flagDaemon := flag.Bool("daemon", false, "Start flatnet in the background")
	flagVerbose := flag.Bool("verbose", false, "Activate verbose logging")
	flagLogfile := flag.String("logfile", "", "Set this to redirect logging into a logfile")
	flagBrokers := flag.String("brokers", "docker:9092,192.168.59.100:9092", "The Kafka brokers to connect to, as a comma separated list")
	flagPrefixes := flag.String("interfaces", "", "Only listen on interfaces having one of the given prefixes. "+
		"Multiple prefixes can be specified as a comma separated list.")

	flag.Parse()

	if *flagDaemon {
		log.Info("Daemonizing process into background now")
		godaemon.MakeDaemon(&godaemon.DaemonAttr{})
	}

	if *flagVerbose {
		log.SetLevel(log.DebugLevel)
	}

	gracefulShutdown := shutdownSignalHandler()

	if *flagLogfile != "" {
		file, err := os.OpenFile(*flagLogfile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0666)
		if err != nil {
			log.WithError(err).Warn("Could not open logfile")
		} else {
			defer file.Close()
			log.SetOutput(file)
		}
	}

	var nameProvider NameProvider = &NoopNameProvider{}
	if *flagConsul != "" {
		var err error
		nameProvider, err = NewConsulNameProvider(*flagConsul)
		if err != nil {
			log.Fatal("Could not initialize consul name provider")
			return
		}
	}

	brokers := strings.Split(*flagBrokers, ",")
	prefixes := strings.Split(*flagPrefixes, ",")

	devices, err := pcap.FindAllDevs()
	if err != nil {
		log.WithError(err).Fatal("Could not list interfaces")
		return
	}

	packages := make(chan *NetPackage, 2048)

	// start the package consumer. It willl take packages from the channel,
	// aggregate them and send them to kafka. The resulting channel will be
	// closed when the consumer finsihes his work.
	producerClosed, err := startPackageConsumer(brokers, nameProvider, packages)
	if err != nil {
		log.WithError(err).Fatal("Could not connect to kafka broker")
		return
	}

	var captures []io.Closer
	for _, device := range devices {
		if shouldCapture(device, prefixes) {
			logger := log.WithField("interface", device.Name)

			logger.Info("Trying interface")
			capture, err := StartTcpCapture(device.Name, packages)
			if err != nil {
				logger.WithError(err).Warn("Could not open interface")
				continue
			}

			logger.WithField("addresses", device.Addresses).Info("Capture started on interface")
			captures = append(captures, capture)
		}
	}

	// wait for the user to press ctrl-c
	<-gracefulShutdown

	for _, capture := range captures {
		capture.Close()
	}

	// close down package stream and kafka
	close(packages)
	<-producerClosed
}

func shouldCapture(device pcap.Interface, prefixes []string) bool {
	matches := false
	for _, prefix := range prefixes {
		matches = matches || strings.HasPrefix(device.Name, prefix)
	}

	return matches
}

func shutdownSignalHandler() <-chan os.Signal {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, os.Kill)

	gracefulSignal := make(chan os.Signal, 1)

	go func() {
		signal := <-signals

		log.Info("Sending graceful shutdown signal, press ctrl-c again to force quit")
		gracefulSignal <- signal

		// wait for force-quit signal
		<-signals
		log.Fatal("Force quitting...")
	}()

	return gracefulSignal
}
