package controller

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"time"

	"github.com/RedHatInsights/cloud-connector/internal/config"
	"github.com/RedHatInsights/cloud-connector/internal/domain"
	"github.com/RedHatInsights/cloud-connector/internal/platform/logger"
	"github.com/RedHatInsights/cloud-connector/internal/platform/queue"

	"github.com/google/uuid"
	kafka "github.com/segmentio/kafka-go"
	"github.com/sirupsen/logrus"
)

type ConnectedClientRecorder interface {
	RecordConnectedClient(context.Context, domain.Identity, domain.AccountID, domain.ClientID, interface{}) error
}

func NewConnectedClientRecorder(impl string, cfg *config.Config) (ConnectedClientRecorder, error) {

	switch impl {
	case "inventory":
		kafkaProducerCfg := &queue.ProducerConfig{
			Brokers:    cfg.InventoryKafkaBrokers,
			Topic:      cfg.InventoryKafkaTopic,
			BatchSize:  cfg.InventoryKafkaBatchSize,
			BatchBytes: cfg.InventoryKafkaBatchBytes,
		}

		kafkaProducer := queue.StartProducer(kafkaProducerCfg)

		connectedClientRecorder := InventoryBasedConnectedClientRecorder{
			KafkaWriter:          kafkaProducer,
			StaleTimestampOffset: cfg.InventoryStaleTimestampOffset,
			ReporterName:         cfg.InventoryReporterName,
		}

		return &connectedClientRecorder, nil
	case "fake":
		return &FakeConnectedClientRecorder{}, nil
	default:
		return nil, errors.New("Invalid ConnectedClientRecorder impl requested")
	}
}

type inventoryMessageEnvelope struct {
	Operation        string      `json:"operation"`
	PlatformMetadata interface{} `json:"platform_metadata"`
	Data             interface{} `json:"data"`
}

type platformMetadata struct {
	RequestID   string `json:"request_id"`
	B64Identity string `json:"b64_identity"`
}

type InventoryBasedConnectedClientRecorder struct {
	KafkaWriter          *kafka.Writer
	StaleTimestampOffset time.Duration
	ReporterName         string
}

func (ibccr *InventoryBasedConnectedClientRecorder) RecordConnectedClient(ctx context.Context, identity domain.Identity, account domain.AccountID, clientID domain.ClientID, canonicalFacts interface{}) error {

	requestID, _ := uuid.NewUUID()

	logger := logger.Log.WithFields(logrus.Fields{"request_id": requestID.String(),
		"account":   account,
		"client_id": clientID})

	staleTimestamp := time.Now().Add(ibccr.StaleTimestampOffset)

	originalHostData := canonicalFacts.(map[string]interface{})

	hostData := cleanupCanonicalFacts(logger, originalHostData)

	hostData["account"] = string(account)
	hostData["stale_timestamp"] = staleTimestamp.UTC().Format("2006-01-02T15:04:05Z07:00")
	hostData["reporter"] = ibccr.ReporterName

	var systemProfile = map[string]string{"rhc_client_id": string(clientID)}
	hostData["system_profile"] = systemProfile

	metadata := platformMetadata{RequestID: requestID.String(), B64Identity: string(identity)}
	envelope := inventoryMessageEnvelope{
		Operation:        "add_host",
		PlatformMetadata: metadata,
		Data:             hostData,
	}

	jsonInventoryMessage, err := json.Marshal(envelope)
	if err != nil {
		logger.WithFields(logrus.Fields{"error": err}).Error("JSON marshal of inventory message failed")
		return err
	}

	go func() {
		metrics.responseKafkaWriterGoRoutineGauge.Inc()
		defer metrics.responseKafkaWriterGoRoutineGauge.Dec()

		err = ibccr.KafkaWriter.WriteMessages(ctx,
			kafka.Message{
				Value: jsonInventoryMessage,
			})

		logger.Debug("Inventory kafka message written")

		if err != nil {
			logger.WithFields(logrus.Fields{"error": err}).Error("Error writing response message to kafka")

			if errors.Is(err, context.Canceled) != true {
				metrics.responseKafkaWriterFailureCounter.Inc()
			}
		} else {
			metrics.responseKafkaWriterSuccessCounter.Inc()
		}
	}()

	return nil
}

func cleanupCanonicalFacts(logger *logrus.Entry, canonicalFacts map[string]interface{}) map[string]interface{} {
	hostData := make(map[string]interface{})

	for key, value := range canonicalFacts {
		if value != nil {
			v := reflect.ValueOf(value)
			switch v.Kind() {
			case reflect.Array, reflect.Slice:
				// Do not pass an empty array to inventory
				if v.Len() > 0 {
					hostData[key] = value
				}
			case reflect.String:
				// Do not pass an empty string to inventory
				if len(v.String()) > 0 {
					hostData[key] = value
				}
			default:
				logger.Debugf("Unknown type in canonical facts map - key: %s, value: %s", key, value)
			}
		}
	}

	return hostData
}

type FakeConnectedClientRecorder struct {
}

func (fccr *FakeConnectedClientRecorder) RecordConnectedClient(ctx context.Context, identity domain.Identity, account domain.AccountID, clientID domain.ClientID, canonicalFacts interface{}) error {
	logger := logger.Log.WithFields(logrus.Fields{"account": account, "client_id": clientID})

	logger.Debug("FAKE: connected client recorder: ", canonicalFacts)

	return nil
}
