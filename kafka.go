package main

import (
	"encoding/json"
	"time"

	"github.com/Shopify/sarama"
	log "github.com/Sirupsen/logrus"
	"github.com/flachnetz/flatnet-agent/capture"
	"github.com/flachnetz/flatnet-agent/discovery"
)

func newKafkaProducer(kafkas []string) (sarama.SyncProducer, error) {
	config := sarama.NewConfig()
	config.Producer.Return.Errors = false
	config.Producer.RequiredAcks = sarama.WaitForLocal       // Only wait for the leader to ack
	config.Producer.Compression = sarama.CompressionSnappy   // Compress messages
	config.Producer.Flush.Frequency = 500 * time.Millisecond // Flush batches every 500ms

	return sarama.NewSyncProducer(kafkas, config)
}

type PacketConsumer struct {
	packets        <-chan capture.Packet
	kafka          sarama.SyncProducer
	nameProvider   discovery.NameProvider
	aggregated     chan *NetPackages
	topic          string
	producerClosed chan struct{}
	closeSignal    chan bool
}

func NewPacketConsumer(brokers []string, nameProvider discovery.NameProvider, packets <-chan capture.Packet) (*PacketConsumer, error) {

	kafka, err := newKafkaProducer(brokers)
	if err != nil {
		return nil, err
	}

	consumer := &PacketConsumer{
		kafka:          kafka,
		packets:        packets,
		nameProvider:   nameProvider,
		aggregated:     make(chan *NetPackages, 8),
		topic:          "flatnet_log",
		producerClosed: make(chan struct{}),
	}

	go consumer.aggregate()
	go consumer.forwardToKafka()

	return consumer, nil
}

func (pc *PacketConsumer) Close() error {
	pc.closeSignal <- true
	return nil
}

func (pc *PacketConsumer) Join() {
	<-pc.producerClosed
}

func (pc *PacketConsumer) aggregate() {
	defer close(pc.aggregated)

	type groupKey struct {
		sourceAddr [4]byte
		sourcePort capture.Port
		targetAddr [4]byte
		targetPort capture.Port
	}

	// aggregate for a few seconds before sending a packet
	aggregationInterval := 2 * time.Second
	ticker := time.NewTicker(aggregationInterval)
	defer ticker.Stop()

	aggregated := make(map[groupKey]*NetPackage)
	for {
		select {
		case <-pc.closeSignal:
			return

		case now := <-ticker.C:
			if len(aggregated) > 0 {
				aggregatedPackagesList := []*NetPackage{}
				for _, p := range aggregated {
					aggregatedPackagesList = append(aggregatedPackagesList, p)
				}

				// reset the aggregation map
				aggregated = make(map[groupKey]*NetPackage)

				result := &NetPackages{
					Timestamp:        now.UnixNano() / int64(time.Millisecond),
					ServicePackages:  aggregatedPackagesList,
					DurationInMillis: int(aggregationInterval / time.Millisecond),
				}

				// send non-blocking, drop packages if consumer queue is full.
				select {
				case pc.aggregated <- result:
				default:
					log.Warn("Could not enqueue aggregated packet, channel full")
				}
			}

		case packet, ok := <-pc.packets:
			if !ok {
				// end of stream reached...
				return
			}

			// build a key to group packets on the same route
			key := groupKey{
				sourcePort: packet.Source.Port,
				targetPort: packet.Target.Port,
			}
			copy(key.sourceAddr[:], packet.Source.Host[:])
			copy(key.targetAddr[:], packet.Target.Host[:])

			// get the packet with this aggregation key
			p, exists := aggregated[key]
			if !exists {
				// packet does not yet exist, create it
				p = &NetPackage{
					Source:      Service{IP: packet.Source.Host.String(), Port: uint16(packet.Source.Port)},
					Destination: Service{IP: packet.Target.Host.String(), Port: uint16(packet.Target.Port)},
					Timestamp:   uint64(packet.Timestamp),
				}

				p.Source.Name = pc.nameProvider.GetName(packet.Source.Host.String(), uint16(packet.Source.Port))
				p.Destination.Name = pc.nameProvider.GetName(packet.Target.Host.String(), uint16(packet.Target.Port))
				aggregated[key] = p
			}

			// update packet.
			p.Len += packet.Length
			p.Count += 1
		}
	}
}

func (pc *PacketConsumer) forwardToKafka() {
	defer func() {
		pc.kafka.Close()
		close(pc.producerClosed)
	}()

	for value := range pc.aggregated {
		if bytes, err := json.Marshal(value); err != nil {
			log.WithError(err).Warn("Could not encode package")

		} else {
			if log.StandardLogger().Level >= log.DebugLevel {
				log.WithField("value", string(bytes)).Debug("Send packet to kafka")
			}

			partition, offset, err := pc.kafka.SendMessage(&sarama.ProducerMessage{
				Topic: pc.topic,
				Key:   nil,
				Value: sarama.ByteEncoder(bytes),
			})

			if err != nil {
				log.WithError(err).Warn("Could not send message to kafka")

			} else if log.StandardLogger().Level >= log.DebugLevel {
				log.WithField("partition", partition).WithField("offset", offset).Debug("Wrote message to broker")
			}
		}
	}
}
