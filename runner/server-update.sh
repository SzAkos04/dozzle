#!/bin/sh
# Builds the fork from ~/dozzle-src (branch custom) and restarts Dozzle from ~/dozzle on the new image.
# Run it over ssh, not from Dozzle's own Update button: the button would cut off the connection
# that reports the result.   ssh szakos@csodabogar2 '~/dozzle-src/runner/server-update.sh'
set -eu
cd ~/dozzle-src
git pull --ff-only origin custom
commit=$(git rev-parse --short HEAD)
docker build --build-arg TAG="v11.4.0-fork.$commit" -t dozzle-custom:"$commit" -t dozzle-custom:latest .
cp runner/update-runner.py ~/update-runner/update-runner.py
systemctl --user restart update-runner
cd ~/dozzle && docker compose up -d
echo "Dozzle fork $commit is running"
