import importlib.util
import os
import stat
import tempfile
import threading
import time
import unittest
import urllib.error
import urllib.request

spec = importlib.util.spec_from_file_location("runner", os.path.join(os.path.dirname(__file__), "update-runner.py"))
runner = importlib.util.module_from_spec(spec)
spec.loader.exec_module(runner)

TOKEN = "t" * 20


def script(dirname, name, body):
    path = os.path.join(dirname, name)
    with open(path, "w") as f:
        f.write("#!/bin/sh\n" + body)
    os.chmod(path, os.stat(path).st_mode | stat.S_IXUSR)
    return path


class RunnerTest(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        cfg = {"listen": "127.0.0.1", "port": 0, "scripts": {
            "ok": script(self.dir.name, "ok.sh", "echo one; echo two\n"),
            "bad": script(self.dir.name, "bad.sh", "echo boom; exit 3\n"),
            "slow": script(self.dir.name, "slow.sh", f"echo run >> {self.dir.name}/runs; sleep 1; echo done\n"),
        }}
        self.srv = runner.make_server(cfg, TOKEN)
        threading.Thread(target=self.srv.serve_forever, daemon=True).start()
        self.base = f"http://127.0.0.1:{self.srv.server_port}"

    def tearDown(self):
        self.srv.shutdown()
        self.dir.cleanup()

    def post(self, path, token=TOKEN):
        req = urllib.request.Request(self.base + path, method="POST", headers={"Authorization": f"Bearer {token}"})
        with urllib.request.urlopen(req) as r:
            return r.read().decode().splitlines()

    def test_output_then_exit_code(self):
        self.assertEqual(self.post("/run/ok"), ["one", "two", "::exit 0"])
        self.assertEqual(self.post("/run/bad"), ["boom", "::exit 3"])

    def test_wrong_token_and_unknown_name(self):
        for path, token, code in [("/run/ok", "x" * 20, 401), ("/run/nope", TOKEN, 404), ("/other", TOKEN, 404)]:
            with self.assertRaises(urllib.error.HTTPError) as e:
                self.post(path, token)
            self.assertEqual(e.exception.code, code)

    def test_two_requests_share_one_run(self):
        out = []
        threads = [threading.Thread(target=lambda: out.append(self.post("/run/slow"))) for _ in range(2)]
        for t in threads:
            t.start()
            time.sleep(0.2)
        for t in threads:
            t.join()
        self.assertEqual(out, [["done", "::exit 0"]] * 2)
        self.assertEqual(open(f"{self.dir.name}/runs").read().count("run"), 1)


if __name__ == "__main__":
    unittest.main()
