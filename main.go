package main

import (
	"flag"
	"os"
	"os/signal"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/VividCortex/godaemon"
	"github.com/flachnetz/flatnet-agent/capture"
	"github.com/flachnetz/flatnet-agent/discovery"
	"github.com/fsouza/go-dockerclient"
	"github.com/google/gopacket/pcap"
	consulapi "github.com/hashicorp/consul/api"
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
	Len         uint32
	Count       int
	Timestamp   uint64
}

func main() {
	flagConsul := flag.String("consul", "", "Address of consul")
	flagDaemon := flag.Bool("daemon", false, "Start flatnet in the background")
	flagVerbose := flag.Bool("verbose", false, "Activate verbose logging")
	flagLogfile := flag.String("logfile", "", "Set this to redirect logging into a logfile")
	flagBrokers := flag.String("brokers", "docker:9092,192.168.59.100:9092", "The Kafka brokers to connect to, as a comma separated list")
	flagPrefixes := flag.String("interfaces", "", "Only listen on interfaces having one of the given prefixes. "+
		"Multiple prefixes can be specified as a comma separated list.")

	flagDocker := flag.String("docker", "", "Docker endpoint. Can be unix:///var/run/docker.sock or tcp://address:port")

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

	nameProvider := discovery.NewNoopNameProvider()

	if *flagConsul != "" {
		config := consulapi.DefaultConfig()
		config.Address = *flagConsul
		client, err := consulapi.NewClient(config)
		if err != nil {
			log.WithError(err).Fatal("Could not initialize consul client")
			return
		}

		nameProvider, err = discovery.NewConsulNameProvider(client)
		if err != nil {
			log.WithError(err).Fatal("Could not initialize consul name provider")
			return
		}
	}

	if *flagDocker != "" {
		client, err := docker.NewClient(*flagDocker)
		if err != nil {
			log.WithError(err).Fatal("Could not create docker client")
			return
		}

		nameProvider = discovery.NewDockerNameProvider(client)
	}

	brokers := strings.Split(*flagBrokers, ",")
	prefixes := strings.Split(*flagPrefixes, ",")

	devices, err := pcap.FindAllDevs()
	if err != nil {
		log.WithError(err).Fatal("Could not list interfaces")
		return
	}

	// All capture go-routines will put their captured packets into this channel.
	packets := make(chan capture.Packet, 2048)

	// Start the package consumer. It will take packages from the channel,
	// aggregate them and send them to kafka. The resulting channel will be
	// closed when the consumer finishes his work.
	consumer, err := NewPacketConsumer(brokers, nameProvider, packets)
	if err != nil {
		log.WithError(err).Fatal("Could not connect to kafka broker")
		return
	}

	var captures []capture.Capture
	for _, device := range devices {
		if shouldCapture(device, prefixes) {
			logger := log.WithField("interface", device.Name)

			logger.Info("Try to open interface")
			capture, err := capture.StartCapture(device.Name, packets)
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

	log.Info("Closing capture devices")
	for _, capture := range captures {
		c := capture
		go c.Close()
	}

	// close down packet stream and kafka
	log.Info("Close packet channel and kafka")
	close(packets)

	log.Info("Waiting for everything to shutdown")
	consumer.Join()
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
