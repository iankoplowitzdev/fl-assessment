#!/bin/bash
set -e

aws --endpoint-url=http://localhost:4566 sqs create-queue \
  --queue-name test-queue \
  --region us-east-1
