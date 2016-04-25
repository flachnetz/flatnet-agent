package main

import (
	"encoding/json"
	"github.com/Shopify/sarama"
	log "github.com/Sirupsen/logrus"
	"time"
)

func newKafkaProducer(kafkas []string) (sarama.SyncProducer, error) {
	config := sarama.NewConfig()
	config.Producer.Return.Errors = false
	config.Producer.RequiredAcks = sarama.WaitForLocal       // Only wait for the leader to ack
	config.Producer.Compression = sarama.CompressionSnappy   // Compress messages
	config.Producer.Flush.Frequency = 500 * time.Millisecond // Flush batches every 500ms

	return sarama.NewSyncProducer(kafkas, config)
}

func forwardToAsyncProducer(values <-chan *NetPackages, producer sarama.SyncProducer) {
	for value := range values {
		if bytes, err := json.Marshal(value); err != nil {
			log.WithError(err).Warn("Could not encode package")
		} else {
			if log.StandardLogger().Level >= log.DebugLevel {
				log.WithField("value", string(bytes)).Debug("Send packet to kafka")
			}

			partition, offset, err := producer.SendMessage(&sarama.ProducerMessage{
				Topic: "flatnet_log",
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

func startPackageConsumer(brokers []string, nameProvider NameProvider, packages <-chan *NetPackage) (<-chan bool, error) {
	producerClosed := make(chan bool, 1)
	if producer, err := newKafkaProducer(brokers); err != nil {
		return nil, err

	} else {
		aggregated := make(chan *NetPackages, 4)

		go func() {
			defer close(aggregated)
			aggregate(packages, aggregated, nameProvider)
		}()

		go func() {
			defer func() {
				producer.Close()
				close(producerClosed)
			}()

			forwardToAsyncProducer(aggregated, producer)
		}()
	}

	return producerClosed, nil
}
