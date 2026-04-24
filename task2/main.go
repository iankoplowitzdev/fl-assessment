package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ses"
)

type WorkflowConfig struct {
	Workflow     WorkflowMeta       `json:"workflow"`
	Jobs         []JobConfig        `json:"jobs"`
	Scoring      ScoringConfig      `json:"scoring"`
	Users        []User             `json:"users"`
	EmailService EmailServiceConfig `json:"email_service"`
	Database     DatabaseConfig     `json:"database"`
}

type DatabaseConfig struct {
	DSN string `json:"dsn"`
}

type WorkflowMeta struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type JobConfig struct {
	ID           string      `json:"id"`
	Type         string      `json:"type"`
	URL          string      `json:"url,omitempty"`
	Dependencies []string    `json:"dependencies"`
	Retry        RetryConfig `json:"retry"`
}

type ScoringConfig struct {
	PassingYardsPerPoint     float64 `json:"passing_yards_per_point"`
	PassingTouchdownPoints   float64 `json:"passing_touchdown_points"`
	InterceptionPoints       float64 `json:"interception_points"`
	RushingYardsPerPoint     float64 `json:"rushing_yards_per_point"`
	RushingTouchdownPoints   float64 `json:"rushing_touchdown_points"`
	ReceivingYardsPerPoint   float64 `json:"receiving_yards_per_point"`
	ReceivingTouchdownPoints float64 `json:"receiving_touchdown_points"`
}

type User struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type EmailServiceConfig struct {
	FromAddress string `json:"from_address"`
	SESEndpoint string `json:"ses_endpoint"`
	Region      string `json:"region"`
}

// JobDeps carries runtime dependencies that jobs may need.
type JobDeps struct {
	Scoring     ScoringConfig
	Users       []User
	SESClient   *ses.Client
	FromAddress string
	DatabaseDSN string
}

func loadConfig(path string) (*WorkflowConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg WorkflowConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return &cfg, nil
}

func setupSES(ctx context.Context, svcCfg EmailServiceConfig) (*ses.Client, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(svcCfg.Region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			envOrDefault("AWS_ACCESS_KEY_ID", "test"),
			envOrDefault("AWS_SECRET_ACCESS_KEY", "test"),
			"",
		)),
	)
	if err != nil {
		return nil, fmt.Errorf("load AWS config: %w", err)
	}
	client := ses.NewFromConfig(cfg, func(o *ses.Options) {
		o.BaseEndpoint = aws.String(svcCfg.SESEndpoint)
	})
	return client, nil
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func buildJob(cfg JobConfig, deps JobDeps) (Job, error) {
	switch cfg.Type {
	case "http_get":
		return newHTTPGetJob(cfg.ID, cfg.URL, cfg.Retry), nil
	case "stat_transformer":
		return newStatTransformerJob(cfg.ID, deps.Scoring, cfg.Retry), nil
	case "emailer":
		return newEmailJob(cfg.ID, deps.Users, deps.SESClient, deps.FromAddress, cfg.Retry), nil
	case "postgres_writer":
		return newPostgresWriterJob(cfg.ID, deps.DatabaseDSN, cfg.Retry), nil
	default:
		return nil, fmt.Errorf("unknown job type: %q", cfg.Type)
	}
}

func main() {
	configPath := "workflow.json"
	notifyPath := "notifications.json"
	if len(os.Args) > 1 {
		configPath = os.Args[1]
	}
	if len(os.Args) > 2 {
		notifyPath = os.Args[2]
	}

	ctx := context.Background()

	cfg, err := loadConfig(configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	bus := &EventBus{}
	bus.Subscribe(NewLoggerSubscriber(log.Default()))

	sesClient, err := setupSES(ctx, cfg.EmailService)
	if err != nil {
		log.Fatalf("setup SES: %v", err)
	}

	notifyCfg, err := loadNotificationConfig(notifyPath)
	if err != nil {
		log.Fatalf("load notification config: %v", err)
	}
	bus.Subscribe(NewNotificationSubscriber(*notifyCfg, sesClient, cfg.EmailService.FromAddress))

	deps := JobDeps{
		Scoring:     cfg.Scoring,
		Users:       cfg.Users,
		SESClient:   sesClient,
		FromAddress: cfg.EmailService.FromAddress,
		DatabaseDSN: cfg.Database.DSN,
	}

	dag := NewDAG(cfg.Workflow.Name, bus)
	for _, jobCfg := range cfg.Jobs {
		job, err := buildJob(jobCfg, deps)
		if err != nil {
			log.Fatalf("build job %q: %v", jobCfg.ID, err)
		}
		dag.AddJob(job, jobCfg.Dependencies)
	}

	if err := dag.Run(ctx); err != nil {
		log.Fatalf("workflow failed: %v", err)
	}
}
