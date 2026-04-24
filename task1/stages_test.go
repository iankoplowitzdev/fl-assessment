package main

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"sync"
	"testing"
)

// ============================================================
// Message helpers
// ============================================================

func basePassingMsg() *Message {
	return &Message{
		GameID:          "game-1",
		PrimaryPlayer:   Player{Name: "Tom Brady", Position: "QB"},
		SecondaryPlayer: &Player{Name: "Randy Moss", Position: "WR"},
		StatType:        "passing",
		Yards:           40,
		Score:           GameScore{HomeTeam: "NE", AwayTeam: "MIA"},
		SQSMessageID:    "msg-1",
	}
}

func baseRushingMsg() *Message {
	return &Message{
		GameID:        "game-1",
		PrimaryPlayer: Player{Name: "Adrian Peterson", Position: "RB"},
		StatType:      "rushing",
		Yards:         15,
		Score:         GameScore{HomeTeam: "MIN", AwayTeam: "GB"},
		SQSMessageID:  "msg-2",
	}
}

// ============================================================
// SchemaValidationStage
// ============================================================

func TestSchemaValidation_ValidPassingPlay(t *testing.T) {
	s := &SchemaValidationStage{}
	if err := s.Process(context.Background(), basePassingMsg()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSchemaValidation_ValidRushingPlay(t *testing.T) {
	s := &SchemaValidationStage{}
	if err := s.Process(context.Background(), baseRushingMsg()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSchemaValidation_MissingGameID(t *testing.T) {
	s := &SchemaValidationStage{}
	msg := basePassingMsg()
	msg.GameID = ""
	if err := s.Process(context.Background(), msg); err == nil {
		t.Fatal("expected error for missing game_id")
	}
}

func TestSchemaValidation_MissingPrimaryPlayerName(t *testing.T) {
	s := &SchemaValidationStage{}
	msg := basePassingMsg()
	msg.PrimaryPlayer.Name = ""
	if err := s.Process(context.Background(), msg); err == nil {
		t.Fatal("expected error for missing primary player name")
	}
}

func TestSchemaValidation_MissingPrimaryPlayerPosition(t *testing.T) {
	s := &SchemaValidationStage{}
	msg := basePassingMsg()
	msg.PrimaryPlayer.Position = ""
	if err := s.Process(context.Background(), msg); err == nil {
		t.Fatal("expected error for missing primary player position")
	}
}

func TestSchemaValidation_PassingNilSecondaryPlayer(t *testing.T) {
	s := &SchemaValidationStage{}
	msg := basePassingMsg()
	msg.SecondaryPlayer = nil
	if err := s.Process(context.Background(), msg); err == nil {
		t.Fatal("expected error when passing play has no secondary player")
	}
}

func TestSchemaValidation_PassingEmptySecondaryPlayerName(t *testing.T) {
	s := &SchemaValidationStage{}
	msg := basePassingMsg()
	msg.SecondaryPlayer = &Player{Name: "", Position: "WR"}
	if err := s.Process(context.Background(), msg); err == nil {
		t.Fatal("expected error when passing play secondary player has empty name")
	}
}

func TestSchemaValidation_PassingEmptySecondaryPlayerPosition(t *testing.T) {
	s := &SchemaValidationStage{}
	msg := basePassingMsg()
	msg.SecondaryPlayer = &Player{Name: "Randy Moss", Position: ""}
	if err := s.Process(context.Background(), msg); err == nil {
		t.Fatal("expected error when passing play secondary player has empty position")
	}
}

func TestSchemaValidation_InvalidStatType(t *testing.T) {
	s := &SchemaValidationStage{}
	msg := baseRushingMsg()
	msg.StatType = "kicking"
	if err := s.Process(context.Background(), msg); err == nil {
		t.Fatal("expected error for invalid stat_type")
	}
}

func TestSchemaValidation_PassingRequiresQB(t *testing.T) {
	s := &SchemaValidationStage{}
	msg := basePassingMsg()
	msg.PrimaryPlayer.Position = "WR"
	if err := s.Process(context.Background(), msg); err == nil {
		t.Fatal("expected error when passing play primary player is not QB")
	}
}

func TestSchemaValidation_RushingRequiresRB(t *testing.T) {
	s := &SchemaValidationStage{}
	msg := baseRushingMsg()
	msg.PrimaryPlayer.Position = "QB"
	if err := s.Process(context.Background(), msg); err == nil {
		t.Fatal("expected error when rushing play primary player is not RB")
	}
}

func TestSchemaValidation_MissingScoreHomeTeam(t *testing.T) {
	s := &SchemaValidationStage{}
	msg := basePassingMsg()
	msg.Score.HomeTeam = ""
	if err := s.Process(context.Background(), msg); err == nil {
		t.Fatal("expected error for missing home team name")
	}
}

func TestSchemaValidation_MissingScoreAwayTeam(t *testing.T) {
	s := &SchemaValidationStage{}
	msg := basePassingMsg()
	msg.Score.AwayTeam = ""
	if err := s.Process(context.Background(), msg); err == nil {
		t.Fatal("expected error for missing away team name")
	}
}

// ============================================================
// FantasyPointTranslationStage
// ============================================================

func testRules() map[string]scoringRule {
	return map[string]scoringRule{
		"passing": {PointsPerYard: 0.04, TouchdownPoints: 4},
		"rushing": {PointsPerYard: 0.1, TouchdownPoints: 6},
	}
}

func TestFantasyPoints_PassingNoTouchdown(t *testing.T) {
	s := &FantasyPointTranslationStage{rules: testRules()}
	msg := basePassingMsg()
	msg.Yards = 100
	msg.Touchdown = false
	if err := s.Process(context.Background(), msg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := 100 * 0.04
	if msg.FantasyPoints != want {
		t.Errorf("got %.4f, want %.4f", msg.FantasyPoints, want)
	}
}

func TestFantasyPoints_PassingWithTouchdown(t *testing.T) {
	s := &FantasyPointTranslationStage{rules: testRules()}
	msg := basePassingMsg()
	msg.Yards = 50
	msg.Touchdown = true
	if err := s.Process(context.Background(), msg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := 50*0.04 + 4.0
	if msg.FantasyPoints != want {
		t.Errorf("got %.4f, want %.4f", msg.FantasyPoints, want)
	}
}

func TestFantasyPoints_RushingNoTouchdown(t *testing.T) {
	s := &FantasyPointTranslationStage{rules: testRules()}
	msg := baseRushingMsg()
	msg.Yards = 80
	msg.Touchdown = false
	if err := s.Process(context.Background(), msg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := 80 * 0.1
	if msg.FantasyPoints != want {
		t.Errorf("got %.4f, want %.4f", msg.FantasyPoints, want)
	}
}

func TestFantasyPoints_RushingWithTouchdown(t *testing.T) {
	s := &FantasyPointTranslationStage{rules: testRules()}
	msg := baseRushingMsg()
	msg.Yards = 10
	msg.Touchdown = true
	if err := s.Process(context.Background(), msg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := 10*0.1 + 6.0
	if msg.FantasyPoints != want {
		t.Errorf("got %.4f, want %.4f", msg.FantasyPoints, want)
	}
}

func TestFantasyPoints_ZeroYards(t *testing.T) {
	s := &FantasyPointTranslationStage{rules: testRules()}
	msg := baseRushingMsg()
	msg.Yards = 0
	msg.Touchdown = false
	if err := s.Process(context.Background(), msg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.FantasyPoints != 0 {
		t.Errorf("got %.4f, want 0", msg.FantasyPoints)
	}
}

func TestFantasyPoints_UnknownStatType(t *testing.T) {
	s := &FantasyPointTranslationStage{rules: testRules()}
	msg := baseRushingMsg()
	msg.StatType = "kicking"
	if err := s.Process(context.Background(), msg); err == nil {
		t.Fatal("expected error for unknown stat_type")
	}
}

// ============================================================
// DatabasePersistenceStage — minimal in-process SQL driver mock
// ============================================================

// mockConn satisfies driver.Conn and driver.ExecerContext so that
// database/sql calls ExecContext directly without going through Prepare.
type mockConn struct {
	rowsAffected int64
	execErr      error
}

func (c *mockConn) Prepare(query string) (driver.Stmt, error) {
	return &mockStmt{conn: c}, nil
}
func (c *mockConn) Close() error              { return nil }
func (c *mockConn) Begin() (driver.Tx, error) { return nil, errors.New("transactions not supported") }

// ExecContext is the fast path used by database/sql when the driver supports it.
func (c *mockConn) ExecContext(_ context.Context, _ string, _ []driver.NamedValue) (driver.Result, error) {
	return c.doExec()
}

func (c *mockConn) doExec() (driver.Result, error) {
	if c.execErr != nil {
		return nil, c.execErr
	}
	return &mockResult{rowsAffected: c.rowsAffected}, nil
}

// mockStmt is the fallback path in case the fast path is not taken.
type mockStmt struct{ conn *mockConn }

func (s *mockStmt) Close() error                                  { return nil }
func (s *mockStmt) NumInput() int                                 { return -1 }
func (s *mockStmt) Exec(_ []driver.Value) (driver.Result, error) { return s.conn.doExec() }
func (s *mockStmt) Query(_ []driver.Value) (driver.Rows, error)  { return nil, errors.New("not supported") }

type mockResult struct{ rowsAffected int64 }

func (r *mockResult) LastInsertId() (int64, error) { return 0, nil }
func (r *mockResult) RowsAffected() (int64, error) { return r.rowsAffected, nil }

type testDriver struct {
	mu       sync.Mutex
	makeConn func() driver.Conn
}

func (d *testDriver) Open(_ string) (driver.Conn, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.makeConn(), nil
}

var (
	dbDrv        *testDriver
	registerOnce sync.Once
)

func newMockDB(t *testing.T, rowsAffected int64, execErr error) *sql.DB {
	t.Helper()
	registerOnce.Do(func() {
		dbDrv = &testDriver{}
		sql.Register("testmock", dbDrv)
	})
	dbDrv.mu.Lock()
	dbDrv.makeConn = func() driver.Conn {
		return &mockConn{rowsAffected: rowsAffected, execErr: execErr}
	}
	dbDrv.mu.Unlock()

	db, err := sql.Open("testmock", "")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestDatabasePersistence_InsertSuccess(t *testing.T) {
	s := &DatabasePersistenceStage{db: newMockDB(t, 1, nil)}
	msg := basePassingMsg()
	msg.FantasyPoints = 5.6
	if err := s.Process(context.Background(), msg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDatabasePersistence_DuplicateSkipped(t *testing.T) {
	s := &DatabasePersistenceStage{db: newMockDB(t, 0, nil)}
	// RowsAffected == 0 is the ON CONFLICT DO NOTHING path; Process should still return nil.
	if err := s.Process(context.Background(), basePassingMsg()); err != nil {
		t.Fatalf("unexpected error for duplicate message: %v", err)
	}
}

func TestDatabasePersistence_DBError(t *testing.T) {
	s := &DatabasePersistenceStage{db: newMockDB(t, 0, errors.New("connection refused"))}
	if err := s.Process(context.Background(), basePassingMsg()); err == nil {
		t.Fatal("expected error when DB exec fails")
	}
}

func TestDatabasePersistence_NilSecondaryPlayer(t *testing.T) {
	s := &DatabasePersistenceStage{db: newMockDB(t, 1, nil)}
	msg := baseRushingMsg() // no SecondaryPlayer
	if err := s.Process(context.Background(), msg); err != nil {
		t.Fatalf("unexpected error with nil secondary player: %v", err)
	}
}

func TestDatabasePersistence_WithSecondaryPlayer(t *testing.T) {
	s := &DatabasePersistenceStage{db: newMockDB(t, 1, nil)}
	if err := s.Process(context.Background(), basePassingMsg()); err != nil {
		t.Fatalf("unexpected error with secondary player: %v", err)
	}
}
