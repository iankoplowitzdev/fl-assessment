#!/bin/bash
set -e

for i in $(seq 1 100); do
  AWS_ACCESS_KEY_ID=test AWS_SECRET_ACCESS_KEY=test \
  aws --endpoint-url=http://localhost:4566 sqs send-message \
    --queue-url http://localhost:4566/000000000000/test-queue \
    --message-body "{\"message\": \"hello world $i\"}" \
    --region us-east-1 \
    --output text &
done

wait
