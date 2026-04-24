CREATE TABLE IF NOT EXISTS stat_scoring_rules (
    stat_type       VARCHAR(50)    PRIMARY KEY,
    points_per_yard NUMERIC(6, 4)  NOT NULL,
    touchdown_points NUMERIC(5, 2) NOT NULL DEFAULT 0
);

INSERT INTO stat_scoring_rules (stat_type, points_per_yard, touchdown_points) VALUES
    ('passing', 0.0400, 6.0),
    ('rushing', 0.1000, 6.0)
ON CONFLICT DO NOTHING;

CREATE TABLE IF NOT EXISTS game_plays (
    id                        SERIAL PRIMARY KEY,
    game_id                   VARCHAR(50)  NOT NULL,
    primary_player_name       VARCHAR(100) NOT NULL,
    primary_player_position   VARCHAR(10)  NOT NULL,
    secondary_player_name     VARCHAR(100),
    secondary_player_position VARCHAR(10),
    yards                     INT          NOT NULL,
    touchdown                 BOOLEAN      NOT NULL DEFAULT false,
    stat_type                 VARCHAR(50)  NOT NULL REFERENCES stat_scoring_rules(stat_type),
    home_team                 VARCHAR(100) NOT NULL,
    away_team                 VARCHAR(100) NOT NULL,
    fantasy_points            NUMERIC(8, 2),
    created_at                TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
