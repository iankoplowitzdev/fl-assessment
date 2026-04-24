package main

import (
	"database/sql"
	"fmt"
	"log"
)

type Stage interface {
	Setup() error
	Process(msg *Message) error
	Teardown() error
}

type SchemaValidationStage struct{}

func (s *SchemaValidationStage) Setup() error {
	log.Println("SchemaValidationStage: setup")
	return nil
}

func (s *SchemaValidationStage) Process(msg *Message) error {
	if msg.GameID == "" {
		return fmt.Errorf("missing game_id")
	}
	if msg.PrimaryPlayer.Name == "" || msg.PrimaryPlayer.Position == "" {
		return fmt.Errorf("missing primary_player fields")
	}
	if msg.StatType == "passing" && (msg.SecondaryPlayer == nil || msg.SecondaryPlayer.Name == "" || msg.SecondaryPlayer.Position == "") {
		return fmt.Errorf("passing play requires a secondary_player (receiver)")
	}
	if msg.StatType != "passing" && msg.StatType != "rushing" {
		return fmt.Errorf("invalid stat_type %q: must be \"passing\" or \"rushing\"", msg.StatType)
	}
	if msg.StatType == "passing" && msg.PrimaryPlayer.Position != "QB" {
		return fmt.Errorf("passing play requires QB as primary player, got %q", msg.PrimaryPlayer.Position)
	}
	if msg.StatType == "rushing" && msg.PrimaryPlayer.Position != "RB" {
		return fmt.Errorf("rushing play requires RB as primary player, got %q", msg.PrimaryPlayer.Position)
	}
	if msg.Score.HomeTeam == "" || msg.Score.AwayTeam == "" {
		return fmt.Errorf("missing score team names")
	}
	log.Printf("SchemaValidationStage: valid game_id=%s stat_type=%s yards=%d touchdown=%v",
		msg.GameID, msg.StatType, msg.Yards, msg.Touchdown)
	return nil
}

func (s *SchemaValidationStage) Teardown() error {
	log.Println("SchemaValidationStage: teardown")
	return nil
}

type FantasyPointTranslationStage struct {
	db *sql.DB
}

func (s *FantasyPointTranslationStage) Setup() error {
	log.Println("FantasyPointTranslationStage: setup")
	return nil
}

func (s *FantasyPointTranslationStage) Process(msg *Message) error {
	var pointsPerYard, touchdownPoints float64
	err := s.db.QueryRow(
		`SELECT points_per_yard, touchdown_points FROM stat_scoring_rules WHERE stat_type = $1`,
		msg.StatType,
	).Scan(&pointsPerYard, &touchdownPoints)
	if err != nil {
		return fmt.Errorf("scoring rule lookup for %q: %w", msg.StatType, err)
	}

	points := float64(msg.Yards) * pointsPerYard
	if msg.Touchdown {
		points += touchdownPoints
	}
	msg.FantasyPoints = points

	log.Printf("FantasyPointTranslationStage: game_id=%s player=%s yards=%d touchdown=%v -> %.2f pts",
		msg.GameID, msg.PrimaryPlayer.Name, msg.Yards, msg.Touchdown, points)
	return nil
}

func (s *FantasyPointTranslationStage) Teardown() error {
	log.Println("FantasyPointTranslationStage: teardown")
	return nil
}

type DatabasePersistenceStage struct {
	db *sql.DB
}

func (s *DatabasePersistenceStage) Setup() error {
	log.Println("DatabasePersistenceStage: setup")
	return nil
}

func (s *DatabasePersistenceStage) Process(msg *Message) error {
	var secondaryName, secondaryPosition sql.NullString
	if msg.SecondaryPlayer != nil {
		secondaryName = sql.NullString{String: msg.SecondaryPlayer.Name, Valid: true}
		secondaryPosition = sql.NullString{String: msg.SecondaryPlayer.Position, Valid: true}
	}

	_, err := s.db.Exec(`
		INSERT INTO game_plays (
			game_id,
			primary_player_name, primary_player_position,
			secondary_player_name, secondary_player_position,
			yards, touchdown, stat_type,
			home_team, away_team,
			fantasy_points
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		msg.GameID,
		msg.PrimaryPlayer.Name, msg.PrimaryPlayer.Position,
		secondaryName, secondaryPosition,
		msg.Yards, msg.Touchdown, msg.StatType,
		msg.Score.HomeTeam, msg.Score.AwayTeam,
		msg.FantasyPoints,
	)
	if err != nil {
		return fmt.Errorf("failed to persist play: %w", err)
	}
	log.Printf("DatabasePersistenceStage: saved game_id=%s player=%s fantasy_points=%.2f",
		msg.GameID, msg.PrimaryPlayer.Name, msg.FantasyPoints)
	return nil
}

func (s *DatabasePersistenceStage) Teardown() error {
	log.Println("DatabasePersistenceStage: teardown")
	return nil
}
