package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
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
	SQSMessageID    string    `json:"-"`
	FantasyPoints   float64   `json:"-"`
}

func processMessage(ctx context.Context, client *sqs.Client, queueURL *string, stages []Stage, msg sqstypes.Message) error {
	var payload Message
	if err := json.Unmarshal([]byte(*msg.Body), &payload); err != nil {
		return err
	}
	payload.SQSMessageID = *msg.MessageId

	for _, s := range stages {
		if err := s.Setup(ctx); err != nil {
			return err
		}
		if err := s.Process(ctx, &payload); err != nil {
			return err
		}
		if err := s.Teardown(ctx); err != nil {
			return err
		}
	}

	_, err := client.DeleteMessage(ctx, &sqs.DeleteMessageInput{
		QueueUrl:      queueURL,
		ReceiptHandle: msg.ReceiptHandle,
	})
	return err
}

func loadScoringRules(ctx context.Context, db *sql.DB) map[string]scoringRule {
	rows, err := db.QueryContext(ctx, `SELECT stat_type, points_per_yard, touchdown_points FROM stat_scoring_rules`)
	if err != nil {
		log.Fatalf("failed to load scoring rules: %v", err)
	}
	defer rows.Close()

	rules := make(map[string]scoringRule)
	for rows.Next() {
		var statType string
		var rule scoringRule
		if err := rows.Scan(&statType, &rule.PointsPerYard, &rule.TouchdownPoints); err != nil {
			log.Fatalf("failed to scan scoring rule: %v", err)
		}
		rules[statType] = rule
	}
	if err := rows.Err(); err != nil {
		log.Fatalf("error iterating scoring rules: %v", err)
	}
	log.Printf("loaded %d scoring rules", len(rules))
	return rules
}

func buildStageRegistry(ctx context.Context, db *sql.DB) map[string]Stage {
	rules := loadScoringRules(ctx, db)
	return map[string]Stage{
		"SCHEMA_VALIDATION":         &SchemaValidationStage{},
		"FANTASY_POINT_TRANSLATION": &FantasyPointTranslationStage{rules: rules},
		"DATABASE_PERSISTENCE":      &DatabasePersistenceStage{db: db},
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

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)
	db.SetConnMaxLifetime(5 * time.Minute)

	log.Println("connected to database")
	return db
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

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

	registry := buildStageRegistry(ctx, db)
	stages := loadStages(registry)

	cfg, err := config.LoadDefaultConfig(ctx,
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

	urlOut, err := client.GetQueueUrl(ctx, &sqs.GetQueueUrlInput{
		QueueName: &queueName,
	})
	if err != nil {
		log.Fatalf("failed to get queue URL: %v", err)
	}
	queueURL := urlOut.QueueUrl
	log.Printf("polling %s", *queueURL)

	var wg sync.WaitGroup
	for {
		out, err := client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
			QueueUrl:            queueURL,
			MaxNumberOfMessages: 10,
			WaitTimeSeconds:     20,
		})
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			log.Printf("error receiving messages: %v", err)
			continue
		}

		for _, msg := range out.Messages {
			wg.Add(1)
			go func(msg sqstypes.Message) {
				defer wg.Done()
				// WithoutCancel so an in-flight message completes its DB write
				// even after the shutdown signal arrives.
				msgCtx := context.WithoutCancel(ctx)
				if err := processMessage(msgCtx, client, queueURL, stages, msg); err != nil {
					log.Printf("message %s left on queue: %v", *msg.MessageId, err)
				}
			}(msg)
		}
		wg.Wait()
	}

	log.Println("shutdown signal received, waiting for in-flight messages")
	wg.Wait()
	log.Println("shutdown complete")
}
