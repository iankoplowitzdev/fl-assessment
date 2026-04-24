#!/bin/bash
set -e

aws --endpoint-url=http://localhost:4566 ses verify-email-identity \
  --email-address noreply@tmrw.example.com \
  --region us-east-1

echo "SES sender identity verified: noreply@tmrw.example.com"
