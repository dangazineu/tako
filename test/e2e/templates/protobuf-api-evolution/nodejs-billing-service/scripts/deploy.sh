#!/bin/bash
# Mock deployment script for nodejs-billing-service
mkdir -p $(dirname $0)
echo "Deployed with API version: $1" > "deployment_log_$1.txt"
echo "$1" > "last_deployed_version.txt"
# Append to log for idempotency testing
echo "$(date '+%Y-%m-%d %H:%M:%S'): Deployed version $1" >> "deployment_history.log"