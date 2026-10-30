#!/usr/bin/env python3
"""Host side of the myGym fork's script updates (see internal/container/script_update.go).

POST /run/<name> with "Authorization: Bearer <token>" runs the script allowlisted under <name> and
answers with its output line by line, then "::exit <code>". Only names from the config run; Dozzle
never sends a command. Two requests for the same name while it runs share one run, so a bulk update of
a compose project's containers does not deploy twice. The script keeps running if the client goes away.

Config (JSON, default ~/.config/update-runner/config.json, or $UPDATE_RUNNER_CONFIG):
  {"listen": "172.17.0.1", "port": 8085, "token_file": "~/.config/update-runner/token",
   "scripts": {"mygym": "/home/szakos/myGym/scripts/server-update.sh"}}
Listen on the docker0 address, not 0.0.0.0 and not 127.0.0.1 (a container cannot reach the latter).
"""
import hmac
import json
import os
import subprocess
import sys
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


class Job:
    def __init__(self, argv):
        self.lines, self.code, self.cond = [], None, threading.Condition()
        self.proc = subprocess.Popen(argv, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, stdin=subprocess.DEVNULL,
                                     text=True, errors="replace", start_new_session=True, cwd=os.path.expanduser("~"))
        threading.Thread(target=self._pump, daemon=True).start()

    def _pump(self):
        for line in self.proc.stdout:
            with self.cond:
                self.lines.append(line.rstrip("\n"))
                self.cond.notify_all()
        code = self.proc.wait()
        with self.cond:
            self.code = code
            self.cond.notify_all()

    def follow(self):
        """Every line from the start, then the exit code."""
        i = 0
        while True:
            with self.cond:
                self.cond.wait_for(lambda: i < len(self.lines) or self.code is not None)
                batch, i = self.lines[i:], len(self.lines)
                done = self.code is not None and i == len(self.lines)
            yield from batch
            if done:
                yield f"::exit {self.code}"
                return


def make_server(cfg, token):
    running, lock = {}, threading.Lock()

    def job_for(name):
        with lock:
            job = running.get(name)
            if job is None or job.code is not None:
                job = running[name] = Job([cfg["scripts"][name]])
            return job

    class Handler(BaseHTTPRequestHandler):
        def log_message(self, fmt, *args):
            sys.stderr.write("%s %s\n" % (self.address_string(), fmt % args))

        def do_POST(self):
            auth = self.headers.get("Authorization", "")
            if not hmac.compare_digest(auth.encode(), f"Bearer {token}".encode()):
                return self.send_error(401)
            name = self.path.removeprefix("/run/") if self.path.startswith("/run/") else None
            if name not in cfg["scripts"]:
                return self.send_error(404)
            self.send_response(200)
            self.send_header("Content-Type", "text/plain; charset=utf-8")
            self.send_header("Cache-Control", "no-store")
            self.end_headers()
            try:
                for line in job_for(name).follow():
                    self.wfile.write((line + "\n").encode())
                    self.wfile.flush()
            except (BrokenPipeError, ConnectionResetError):
                pass  # the script carries on

    return ThreadingHTTPServer((cfg["listen"], cfg["port"]), Handler)


def main():
    path = os.path.expanduser(os.environ.get("UPDATE_RUNNER_CONFIG", "~/.config/update-runner/config.json"))
    cfg = json.load(open(path))
    token = open(os.path.expanduser(cfg["token_file"])).read().strip()
    if len(token) < 16:
        sys.exit("token_file must hold a token of at least 16 characters")
    for name, script in cfg["scripts"].items():
        if not os.access(script, os.X_OK):
            sys.exit(f"script {name!r}: {script} is not an executable file")
    srv = make_server(cfg, token)
    print(f"update-runner on {cfg['listen']}:{srv.server_port}, scripts: {', '.join(cfg['scripts'])}", flush=True)
    srv.serve_forever()


if __name__ == "__main__":
    main()
