package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
	_ "github.com/lib/pq"
)

type Player struct {
	Name     string `json:"name"`
	Position string `json:"position"`
}

type GameScore struct {
	HomeTeam string `json:"home_team"`
	AwayTeam string `json:"away_team"`
}

type Message struct {
	GameID          string    `json:"game_id"`
	PrimaryPlayer   Player    `json:"primary_player"`
	SecondaryPlayer *Player   `json:"secondary_player"`
	Yards           int       `json:"yards"`
	Touchdown       bool      `json:"touchdown"`
	StatType        string    `json:"stat_type"`
	Score           GameScore `json:"score"`
	FantasyPoints   float64   `json:"-"`
}

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

func buildStageRegistry(db *sql.DB) map[string]Stage {
	return map[string]Stage{
		"SCHEMA_VALIDATION":        &SchemaValidationStage{},
		"FANTASY_POINT_TRANSLATION": &FantasyPointTranslationStage{db: db},
		"DATABASE_PERSISTENCE":     &DatabasePersistenceStage{db: db},
	}
}

func loadStages(registry map[string]Stage) []Stage {
	names := strings.Split(os.Getenv("STAGES"), ",")
	var stages []Stage
	for _, name := range names {
		name = strings.TrimSpace(name)
		if s, ok := registry[name]; ok {
			stages = append(stages, s)
		} else if name != "" {
			log.Printf("unknown stage: %s", name)
		}
	}
	return stages
}

func connectDB() *sql.DB {
	host := os.Getenv("DB_HOST")
	if host == "" {
		host = "localhost"
	}
	port := os.Getenv("DB_PORT")
	if port == "" {
		port = "5432"
	}
	user := os.Getenv("DB_USER")
	if user == "" {
		user = "postgres"
	}
	password := os.Getenv("DB_PASSWORD")
	if password == "" {
		password = "postgres"
	}
	dbName := os.Getenv("DB_NAME")
	if dbName == "" {
		dbName = "nfl_stats"
	}

	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbName)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("failed to open DB: %v", err)
	}

	for i := 0; i < 10; i++ {
		if err = db.Ping(); err == nil {
			break
		}
		log.Printf("waiting for DB (attempt %d/10): %v", i+1, err)
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		log.Fatalf("failed to connect to DB after retries: %v", err)
	}

	log.Println("connected to database")
	return db
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

	db := connectDB()
	defer db.Close()

	registry := buildStageRegistry(db)
	stages := loadStages(registry)

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
