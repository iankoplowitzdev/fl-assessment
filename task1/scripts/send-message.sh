#!/bin/bash
set -e

GAMES=("KC_vs_BUF" "PHI_vs_DAL" "SF_vs_LAR")
HOME_TEAMS=("Kansas City Chiefs" "Philadelphia Eagles" "San Francisco 49ers")
AWAY_TEAMS=("Buffalo Bills" "Dallas Cowboys" "Los Angeles Rams")

QB_NAMES=("Patrick Mahomes" "Josh Allen" "Jalen Hurts" "Dak Prescott" "Brock Purdy" "Matthew Stafford")
RB_NAMES=("Isiah Pacheco" "James Cook" "Saquon Barkley" "Tony Pollard" "Christian McCaffrey" "Kyren Williams")

for i in $(seq 1 100); do
  GAME_IDX=$(( (i - 1) % 3 ))
  PLAYER_IDX=$(( (i - 1) % 6 ))
  YARDS=$(( (RANDOM % 50) + 1 ))
  TOUCHDOWN="false"
  if (( RANDOM % 10 == 0 )); then
    TOUCHDOWN="true"
  fi

  GAME_DATE=$(date +%Y%m%d)
  GAME_ID="${GAMES[$GAME_IDX]}_$GAME_DATE"

  if (( i % 3 == 0 )); then
    STAT_TYPE="rushing"
    PRIMARY_NAME="${RB_NAMES[$PLAYER_IDX]}"
    PRIMARY_POS="RB"
    SECONDARY_FIELD="null"
  else
    STAT_TYPE="passing"
    PRIMARY_NAME="${QB_NAMES[$PLAYER_IDX]}"
    PRIMARY_POS="QB"
    SECONDARY_FIELD=$(printf '{"name": "%s", "position": "RB"}' "${RB_NAMES[$PLAYER_IDX]}")
  fi

  MSG=$(printf '{
  "game_id": "%s",
  "primary_player": {"name": "%s", "position": "%s"},
  "secondary_player": %s,
  "yards": %d,
  "touchdown": %s,
  "stat_type": "%s",
  "score": {
    "home_team": "%s",
    "away_team": "%s"
  }
}' "$GAME_ID" "$PRIMARY_NAME" "$PRIMARY_POS" "$SECONDARY_FIELD" \
     "$YARDS" "$TOUCHDOWN" "$STAT_TYPE" \
     "${HOME_TEAMS[$GAME_IDX]}" "${AWAY_TEAMS[$GAME_IDX]}")

  AWS_ACCESS_KEY_ID=test AWS_SECRET_ACCESS_KEY=test \
  aws --endpoint-url=http://localhost:4566 sqs send-message \
    --queue-url http://localhost:4566/000000000000/test-queue \
    --message-body "$MSG" \
    --region us-east-1 \
    --output text &
done

