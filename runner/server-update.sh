#!/bin/sh
# Builds the fork from ~/dozzle-src (branch custom) and restarts Dozzle from ~/dozzle on the new image.
# Run it over ssh, not from Dozzle's own Update button: the button would cut off the connection
# that reports the result.   ssh szakos@csodabogar2 '~/dozzle-src/runner/server-update.sh'
set -eu
cd ~/dozzle-src
git pull --ff-only origin custom
commit=$(git rev-parse --short HEAD)
# The agent certificate the Dockerfile copies is not in git (see the Makefile); any key pair will do.
[ -f shared_key.pem ] || openssl genpkey -algorithm Ed25519 -out shared_key.pem
[ -f shared_cert.pem ] || { openssl req -new -key shared_key.pem -out shared_request.csr -subj "/O=Dozzle" &&
  openssl x509 -req -in shared_request.csr -signkey shared_key.pem -out shared_cert.pem -days 1825 && rm shared_request.csr; }
docker build --build-arg TAG="v11.4.0-fork.$commit" -t dozzle-custom:"$commit" -t dozzle-custom:latest .
cp runner/update-runner.py ~/update-runner/update-runner.py
systemctl --user restart update-runner
cd ~/dozzle && docker compose up -d
echo "Dozzle fork $commit is running"
