#!/bin/bash
set -e

echo "Starting long-running process..."
# Simulate a longer process by sleeping for a few seconds
sleep 3
echo "Long process completed successfully" > long-process.log
echo "long-process-done"