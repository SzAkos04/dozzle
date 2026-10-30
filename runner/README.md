# myGym fork: update by script

Upstream Dozzle's Update button pulls an image and swaps the container. That is wrong for a compose
project built from a checkout (myGym): it replaced the fork's web image with upstream's. In this fork a
container labelled

    dev.dozzle.update.script: mygym

is updated by running the host script named `mygym` instead. Dozzle asks the runner in this folder, the
runner runs the script from its allowlist, and the output goes into Dozzle's own log (open the `dozzle`
container). Containers without the label update exactly as upstream. Containers with it are also no
longer checked against a registry, since a locally built image says nothing about the checkout.

Dozzle needs two environment variables:

    DOZZLE_UPDATE_RUNNER=http://172.17.0.1:8085      # docker0 address of the host
    DOZZLE_UPDATE_RUNNER_TOKEN=<same token as the runner's token_file>

## Runner on the host

`update-runner.py` is Python stdlib only. Install:

    mkdir -p ~/update-runner ~/.config/update-runner ~/.config/systemd/user
    cp update-runner.py ~/update-runner/ && cp update-runner.service ~/.config/systemd/user/
    (umask 077; head -c 24 /dev/urandom | base64 | tr -d '/+=' > ~/.config/update-runner/token)
    cat > ~/.config/update-runner/config.json <<'EOF'
    {"listen": "172.17.0.1", "port": 8085, "token_file": "~/.config/update-runner/token",
     "scripts": {"mygym": "/home/szakos/myGym/scripts/server-update.sh"}}
    EOF
    systemctl --user daemon-reload && systemctl --user enable --now update-runner

Only the names in `scripts` run, with no arguments. Two requests for one name during a run share it, so
updating both of a project's containers deploys once. A script keeps running if Dozzle goes away.

Tests: `python3 runner/test_update_runner.py`, and `go test ./internal/container/ ./internal/imagecheck/`.

## Updating the fork itself

`server-update.sh` builds this checkout and restarts Dozzle. Run it over ssh.
