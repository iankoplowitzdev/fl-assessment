CREATE TABLE IF NOT EXISTS fantasy_results (
    id          SERIAL PRIMARY KEY,
    week        INT          NOT NULL,
    season      INT          NOT NULL,
    game_id     TEXT         NOT NULL,
    home_team   TEXT         NOT NULL,
    away_team   TEXT         NOT NULL,
    player_name TEXT         NOT NULL,
    player_team TEXT         NOT NULL,
    position    TEXT         NOT NULL,
    points      NUMERIC(8,2) NOT NULL,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
