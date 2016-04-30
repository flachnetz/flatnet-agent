package main

import (
	"os"
	"os/signal"

	"gopkg.in/alecthomas/kingpin.v2"

	"regexp"

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
	flagConsul := kingpin.Flag("consul", "Address of consul.").TCP()
	flagDaemon := kingpin.Flag("daemon", "Start the agent as a daemon.").Bool()
	flagVerbose := kingpin.Flag("verbose", "Enable verbose logging.").Short('v').Bool()
	flagLogfile := kingpin.Flag("logfile", "Set this to redirect logging into a logfile.").String()
	flagBrokers := kingpin.Flag("kafka", "Address of kafka brokers to connect to. Can be specified multiple times.").Required().TCPList()
	flagInterfacePattern := kingpin.Flag("interface", "Regular expression to match against interfaces to be captured.").Default("^(eth|en|docker)[0-9]+$").Regexp()
	flagDocker := kingpin.Flag("docker", "Docker endpoint. Can be unix:///var/run/docker.sock or tcp://address:port.").String()
	// flagMultiNameProvider := kingpin.Flag("multi", "").Bool()

	kingpin.Parse()

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

	if *flagConsul != nil {
		config := consulapi.DefaultConfig()
		config.Address = (*flagConsul).String()
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

	//if *flagMultiNameProvider {
	//	// read this from config file later.
	//
	//	var err error
	//	nameProvider, err = discovery.NewMultiNameProvider(discovery.MultiNameProviderConfig{
	//		"172.19.0.0/16":     discovery.NewConstantNameProvider("ask docker!"),
	//		"10.0.0.0/8::16000": discovery.NewConstantNameProvider("maybe ask consul for ports < 16000"),
	//		"10.2.3.4/32:16000": discovery.NewConstantNameProvider("cs"),
	//	})
	//
	//	if err != nil {
	//		log.WithError(err).Fatal("Could not initialize multi name provider")
	//		return
	//	}
	//}

	var brokers []string
	for _, brokerAddress := range *flagBrokers {
		brokers = append(brokers, brokerAddress.String())
	}

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
		if shouldCapture(device, *flagInterfacePattern) {
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

func shouldCapture(device pcap.Interface, pattern *regexp.Regexp) bool {
	return pattern.MatchString(device.Name)
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
