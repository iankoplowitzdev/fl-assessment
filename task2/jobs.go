package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ses"
	sestypes "github.com/aws/aws-sdk-go-v2/service/ses/types"
	"github.com/jackc/pgx/v5"
)

// ─── Status ───────────────────────────────────────────────────────────────────

type Status string

const (
	Pending   Status = "PENDING"
	Running   Status = "RUNNING"
	Succeeded Status = "SUCCEEDED"
	Failed    Status = "FAILED"
	Cancelled Status = "CANCELLED"
)

// ─── Retry ────────────────────────────────────────────────────────────────────

type RetryConfig struct {
	Enabled       bool    `json:"enabled"`
	MaxAttempts   int     `json:"max_attempts"`
	BackoffScalar float64 `json:"backoff_scalar"`
}

// ─── Job interface ────────────────────────────────────────────────────────────

type Job interface {
	ID() string
	Run(ctx context.Context, input any) (any, error)
	GetStatus() Status
	Cancel()
	RetryPolicy() RetryConfig
}

// ─── BaseJob ──────────────────────────────────────────────────────────────────

type BaseJob struct {
	id          string
	status      Status
	retryPolicy RetryConfig
	mu          sync.RWMutex
}

func (b *BaseJob) ID() string { return b.id }

func (b *BaseJob) GetStatus() Status {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.status
}

func (b *BaseJob) setStatus(s Status) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.status = s
}

func (b *BaseJob) Cancel() { b.setStatus(Cancelled) }

func (b *BaseJob) RetryPolicy() RetryConfig { return b.retryPolicy }

// ─── NFL data types ───────────────────────────────────────────────────────────

type NFLScores struct {
	Week   int    `json:"week"`
	Season int    `json:"season"`
	Games  []Game `json:"games"`
}

type Game struct {
	GameID   string   `json:"game_id"`
	HomeTeam string   `json:"home_team"`
	AwayTeam string   `json:"away_team"`
	Players  []Player `json:"players"`
}

type Player struct {
	Name     string      `json:"name"`
	Team     string      `json:"team"`
	Position string      `json:"position"`
	Stats    PlayerStats `json:"stats"`
}

type PlayerStats struct {
	PassingYards        int `json:"passing_yards"`
	PassingTouchdowns   int `json:"passing_touchdowns"`
	Interceptions       int `json:"interceptions"`
	RushingYards        int `json:"rushing_yards"`
	RushingTouchdowns   int `json:"rushing_touchdowns"`
	ReceivingYards      int `json:"receiving_yards"`
	ReceivingTouchdowns int `json:"receiving_touchdowns"`
	Receptions          int `json:"receptions"`
}

// ─── Transformer result types ─────────────────────────────────────────────────

type TransformerResult struct {
	Week   int           `json:"week"`
	Season int           `json:"season"`
	Games  []GameSummary `json:"games"`
}

type GameSummary struct {
	GameID   string                `json:"game_id"`
	HomeTeam string                `json:"home_team"`
	AwayTeam string                `json:"away_team"`
	Players  []PlayerFantasyPoints `json:"players"`
}

type PlayerFantasyPoints struct {
	Name     string  `json:"name"`
	Team     string  `json:"team"`
	Position string  `json:"position"`
	Points   float64 `json:"fantasy_points"`
}

// ─── HTTP GET Job ─────────────────────────────────────────────────────────────

type HTTPGetJob struct {
	BaseJob
	url    string
	client *http.Client
}

func newHTTPGetJob(id, url string, retry RetryConfig) *HTTPGetJob {
	return &HTTPGetJob{
		BaseJob: BaseJob{id: id, status: Pending, retryPolicy: retry},
		url:     url,
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

func (j *HTTPGetJob) Run(ctx context.Context, _ any) (any, error) {
	j.setStatus(Running)
	data, err := j.fetch(ctx)
	if err != nil {
		j.setStatus(Failed)
		return nil, err
	}
	j.setStatus(Succeeded)
	return data, nil
}

func (j *HTTPGetJob) fetch(ctx context.Context) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, j.url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := j.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// ─── Stat Transformer Job ─────────────────────────────────────────────────────

type StatTransformerJob struct {
	BaseJob
	scoring ScoringConfig
}

func newStatTransformerJob(id string, scoring ScoringConfig, retry RetryConfig) *StatTransformerJob {
	return &StatTransformerJob{
		BaseJob: BaseJob{id: id, status: Pending, retryPolicy: retry},
		scoring: scoring,
	}
}

func (j *StatTransformerJob) Run(_ context.Context, input any) (any, error) {
	j.setStatus(Running)

	raw, ok := input.([]byte)
	if !ok {
		j.setStatus(Failed)
		return nil, fmt.Errorf("expected []byte input, got %T", input)
	}

	var scores NFLScores
	if err := json.Unmarshal(raw, &scores); err != nil {
		j.setStatus(Failed)
		return nil, fmt.Errorf("unmarshal NFL scores: %w", err)
	}

	result := TransformerResult{
		Week:   scores.Week,
		Season: scores.Season,
		Games:  make([]GameSummary, 0, len(scores.Games)),
	}

	for _, game := range scores.Games {
		summary := GameSummary{
			GameID:   game.GameID,
			HomeTeam: game.HomeTeam,
			AwayTeam: game.AwayTeam,
			Players:  make([]PlayerFantasyPoints, 0, len(game.Players)),
		}
		for _, p := range game.Players {
			summary.Players = append(summary.Players, j.toFantasyPoints(p))
		}
		result.Games = append(result.Games, summary)
	}

	j.setStatus(Succeeded)
	return result, nil
}

func (j *StatTransformerJob) toFantasyPoints(p Player) PlayerFantasyPoints {
	sc := j.scoring
	s := p.Stats
	pts := 0.0
	pts += float64(s.PassingYards) / sc.PassingYardsPerPoint
	pts += float64(s.PassingTouchdowns) * sc.PassingTouchdownPoints
	pts += float64(s.Interceptions) * sc.InterceptionPoints
	pts += float64(s.RushingYards) / sc.RushingYardsPerPoint
	pts += float64(s.RushingTouchdowns) * sc.RushingTouchdownPoints
	pts += float64(s.ReceivingYards) / sc.ReceivingYardsPerPoint
	pts += float64(s.ReceivingTouchdowns) * sc.ReceivingTouchdownPoints
	return PlayerFantasyPoints{
		Name:     p.Name,
		Team:     p.Team,
		Position: p.Position,
		Points:   math.Round(pts*100) / 100,
	}
}

// ─── Email Job ────────────────────────────────────────────────────────────────

type EmailJob struct {
	BaseJob
	users       []User
	sesClient   *ses.Client
	fromAddress string
}

func newEmailJob(id string, users []User, sesClient *ses.Client, fromAddress string, retry RetryConfig) *EmailJob {
	return &EmailJob{
		BaseJob:     BaseJob{id: id, status: Pending, retryPolicy: retry},
		users:       users,
		sesClient:   sesClient,
		fromAddress: fromAddress,
	}
}

// Run emails all users concurrently via SES, waiting for all goroutines to finish.
func (j *EmailJob) Run(ctx context.Context, input any) (any, error) {
	j.setStatus(Running)

	result, ok := input.(TransformerResult)
	if !ok {
		j.setStatus(Failed)
		return nil, fmt.Errorf("expected TransformerResult input, got %T", input)
	}

	subject := fmt.Sprintf("Fantasy Football Results — Week %d, %d Season", result.Week, result.Season)
	body := formatEmailBody(result)

	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		errs []error
	)
	for _, u := range j.users {
		wg.Add(1)
		go func(user User) {
			defer wg.Done()
			if err := j.sendEmail(ctx, user, subject, body); err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("send to %s: %w", user.Email, err))
				mu.Unlock()
			}
		}(u)
	}
	wg.Wait()

	if len(errs) > 0 {
		j.setStatus(Failed)
		return nil, errs[0]
	}
	j.setStatus(Succeeded)
	return nil, nil
}

func (j *EmailJob) sendEmail(ctx context.Context, u User, subject, body string) error {
	_, err := j.sesClient.SendEmail(ctx, &ses.SendEmailInput{
		Source: aws.String(j.fromAddress),
		Destination: &sestypes.Destination{
			ToAddresses: []string{u.Email},
		},
		Message: &sestypes.Message{
			Subject: &sestypes.Content{Data: aws.String(subject)},
			Body: &sestypes.Body{
				Text: &sestypes.Content{Data: aws.String(body)},
			},
		},
	})
	if err != nil {
		return err
	}
	log.Printf("[emailer] ✉  Sent to %s <%s>", u.Name, u.Email)
	return nil
}

func formatEmailBody(r TransformerResult) string {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "Fantasy Football Results — Week %d, %d Season\n", r.Week, r.Season)
	fmt.Fprintln(&buf, strings.Repeat("─", 60))
	for _, game := range r.Games {
		fmt.Fprintf(&buf, "\n%s vs %s\n", game.HomeTeam, game.AwayTeam)
		for _, p := range game.Players {
			fmt.Fprintf(&buf, "  %-25s (%-2s, %s)  %6.2f pts\n",
				p.Name, p.Position, p.Team, p.Points)
		}
	}
	return buf.String()
}

// ─── Postgres Writer Job ──────────────────────────────────────────────────────

type PostgresWriterJob struct {
	BaseJob
	dsn string
}

func newPostgresWriterJob(id, dsn string, retry RetryConfig) *PostgresWriterJob {
	return &PostgresWriterJob{
		BaseJob: BaseJob{id: id, status: Pending, retryPolicy: retry},
		dsn:     dsn,
	}
}

func (j *PostgresWriterJob) Run(ctx context.Context, input any) (any, error) {
	j.setStatus(Running)

	result, ok := input.(TransformerResult)
	if !ok {
		j.setStatus(Failed)
		return nil, fmt.Errorf("expected TransformerResult input, got %T", input)
	}

	conn, err := pgx.Connect(ctx, j.dsn)
	if err != nil {
		j.setStatus(Failed)
		return nil, fmt.Errorf("connect to postgres: %w", err)
	}
	defer conn.Close(ctx)

	for _, game := range result.Games {
		for _, p := range game.Players {
			_, err := conn.Exec(ctx, `
				INSERT INTO fantasy_results
					(week, season, game_id, home_team, away_team, player_name, player_team, position, points)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
				result.Week, result.Season,
				game.GameID, game.HomeTeam, game.AwayTeam,
				p.Name, p.Team, p.Position, p.Points,
			)
			if err != nil {
				j.setStatus(Failed)
				return nil, fmt.Errorf("insert player %s: %w", p.Name, err)
			}
		}
	}

	log.Printf("[postgres_writer] wrote %d games (week %d, season %d)", len(result.Games), result.Week, result.Season)
	j.setStatus(Succeeded)
	return nil, nil
}
