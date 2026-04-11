#!/usr/bin/env bash
# Test script for runx stdin/overlay detection
# Tests: password masking, confirm dialog, plain input prompt

echo "=== runx stdin overlay test ==="
echo ""

# 1. Generic input
echo "Enter your name:"
read -r NAME
echo "Hello, $NAME!"
echo ""

# 2. Confirm prompt
echo "Continue? [y/n]"
read -r CONFIRM
echo "You said: $CONFIRM"
echo ""

# 3. Password (masked input)
echo "Enter password:"
read -rs PASSWORD
echo "Password received (length: ${#PASSWORD})"
echo ""

# 4. SSH-style host key confirm
echo "Are you sure you want to continue connecting (yes/no/[fingerprint])?"
read -r KEY_CONFIRM
echo "Key confirm: $KEY_CONFIRM"
echo ""

echo "=== done ==="
