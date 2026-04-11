#!/usr/bin/env bash
# Mock SSH session — mimics exact SSH output format for runx overlay testing.
# No real SSH needed. Tests host key confirm, password prompt, passphrase prompt.

HOST="testuser@192.168.1.100"

echo ""
echo "=== mock SSH: host key prompt ==="
echo ""
echo "The authenticity of host '192.168.1.100 (192.168.1.100)' can't be established."
echo "ED25519 key fingerprint is SHA256:abc123xyz/fakefingerprint+AAAA."
echo "This key is not known by any other names."
printf "Are you sure you want to continue connecting (yes/no/[fingerprint])? "
read -r HOST_CONFIRM
echo ""
if [[ "$HOST_CONFIRM" != "yes" ]]; then
    echo "Host key verification failed."
    exit 1
fi
echo "Warning: Permanently added '192.168.1.100' (ED25519) to the list of known hosts."
echo ""

echo "=== mock SSH: password prompt ==="
echo ""
printf "%s's password: " "$HOST"
read -rs PASSWORD
echo ""
if [[ "$PASSWORD" != "hunter2" ]]; then
    echo "Permission denied, please try again."
    printf "%s's password: " "$HOST"
    read -rs PASSWORD2
    echo ""
    if [[ "$PASSWORD2" != "hunter2" ]]; then
        echo "Permission denied (publickey,password)."
        exit 1
    fi
fi
echo ""

echo "=== mock SSH: passphrase prompt ==="
echo ""
printf "Enter passphrase for key '/home/lucas/.ssh/id_ed25519': "
read -rs PASSPHRASE
echo ""
echo ""

echo "=== connected ==="
echo "Linux testhost 6.1.0 #1 SMP x86_64 GNU/Linux"
echo ""
echo "Last login: Thu Apr 10 14:00:00 2026 from 192.168.1.1"
echo ""
echo "testuser@testhost:~\$ echo hello"
echo "hello"
echo "testuser@testhost:~\$ exit"
echo "logout"
echo "Connection to 192.168.1.100 closed."
