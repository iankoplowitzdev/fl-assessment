package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

func processMessage(client *sqs.Client, queueURL *string, stages []Stage, msg sqstypes.Message) error {
	var payload Message
	if err := json.Unmarshal([]byte(*msg.Body), &payload); err != nil {
		return err
	}

	for _, s := range stages {
		if err := s.Setup(); err != nil {
			return err
		}
		if err := s.Process(&payload); err != nil {
			return err
		}
		if err := s.Teardown(); err != nil {
			return err
		}
	}

	_, err := client.DeleteMessage(context.Background(), &sqs.DeleteMessageInput{
		QueueUrl:      queueURL,
		ReceiptHandle: msg.ReceiptHandle,
	})
	return err
}

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

		var wg sync.WaitGroup
		for _, msg := range out.Messages {
			wg.Add(1)
			go func(msg sqstypes.Message) {
				defer wg.Done()
				if err := processMessage(client, queueURL, stages, msg); err != nil {
					log.Printf("message %s left on queue: %v", *msg.MessageId, err)
				}
			}(msg)
		}
		wg.Wait()
	}
}
