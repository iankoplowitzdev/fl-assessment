package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

type Message struct {
	Message string `json:"message"`
}

var stageRegistry = map[string]Stage{
	"SCHEMA_VALIDATION":   &SchemaValidationStage{},
	"FIELD_TRANSFORMATION": &FieldTransformationStage{},
	"DEDUPLICATION":       &DeduplicationStage{},
}

func loadStages() []Stage {
	names := strings.Split(os.Getenv("STAGES"), ",")
	var stages []Stage
	for _, name := range names {
		name = strings.TrimSpace(name)
		if s, ok := stageRegistry[name]; ok {
			stages = append(stages, s)
		} else if name != "" {
			log.Printf("unknown stage: %s", name)
		}
	}
	return stages
}

func main() {
	endpoint := os.Getenv("SQS_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://localhost:4566"
	}
	queueName := os.Getenv("SQS_QUEUE_NAME")
	if queueName == "" {
		queueName = "test-queue"
	}

	stages := loadStages()

	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion("us-east-1"),
		config.WithEndpointResolverWithOptions(
			aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
				return aws.Endpoint{URL: endpoint, HostnameImmutable: true}, nil
			}),
		),
	)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	client := sqs.NewFromConfig(cfg)

	urlOut, err := client.GetQueueUrl(context.Background(), &sqs.GetQueueUrlInput{
		QueueName: &queueName,
	})
	if err != nil {
		log.Fatalf("failed to get queue URL: %v", err)
	}
	queueURL := urlOut.QueueUrl
	log.Printf("polling %s", *queueURL)

	for {
		out, err := client.ReceiveMessage(context.Background(), &sqs.ReceiveMessageInput{
			QueueUrl:            queueURL,
			MaxNumberOfMessages: 10,
			WaitTimeSeconds:     20,
		})
		if err != nil {
			log.Printf("error receiving messages: %v", err)
			continue
		}

		for _, msg := range out.Messages {
			var payload Message
			if err := json.Unmarshal([]byte(*msg.Body), &payload); err != nil {
				log.Printf("failed to deserialize message %s: %v", *msg.MessageId, err)
			} else {
				for _, s := range stages {
					if err := s.Setup(); err != nil {
						log.Printf("stage setup failed for message %s: %v", *msg.MessageId, err)
						continue
					}
					if err := s.Process(&payload); err != nil {
						log.Printf("stage process failed for message %s: %v", *msg.MessageId, err)
					}
					if err := s.Teardown(); err != nil {
						log.Printf("stage teardown failed for message %s: %v", *msg.MessageId, err)
					}
				}
			}

			_, err := client.DeleteMessage(context.Background(), &sqs.DeleteMessageInput{
				QueueUrl:      queueURL,
				ReceiptHandle: msg.ReceiptHandle,
			})
			if err != nil {
				log.Printf("error deleting message %s: %v", *msg.MessageId, err)
			}
		}
	}
}
