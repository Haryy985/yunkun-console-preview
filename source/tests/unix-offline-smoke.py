"""Test a clean release package using synthetic credentials and local HTTP only."""

import hashlib
import json
import os
from pathlib import Path
import secrets
import socket
import subprocess
import sys
import time
import urllib.error
import urllib.request


root = Path(sys.argv[1]).resolve()
result_path = Path(sys.argv[2]).resolve()
base = "http://127.0.0.1:53513"
opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
checks = []
process = None
output = None


def request(path, data=None, token=None):
    headers = {}
    if data is not None:
        headers["Content-Type"] = "application/json"
    if token:
        headers["Authorization"] = "Bearer " + token
    req = urllib.request.Request(
        base + path,
        data=json.dumps(data).encode() if data is not None else None,
        headers=headers,
    )
    with opener.open(req, timeout=3) as response:
        return response.read()


def api(path, data=None, token=None):
    return json.loads(request(path, data, token))


def start():
    global process, output
    output = (root.parent / "smoke-launcher.log").open("ab")
    env = os.environ.copy()
    # Deliberately invalid proxy settings catch accidental proxying of local readiness.
    env.update(HTTP_PROXY="http://127.0.0.1:1", HTTPS_PROXY="http://127.0.0.1:1")
    process = subprocess.Popen(
        [str(root / "start.sh"), "--no-browser", "--port", "53513",
         "--database-port", "55435"],
        cwd=root, env=env, stdout=output, stderr=subprocess.STDOUT,
    )
    for _ in range(160):
        if process.poll() is not None:
            raise RuntimeError("launcher exited: " + (root.parent / "smoke-launcher.log").read_text(errors="replace"))
        try:
            health = api("/api/health")
            if health.get("ready"):
                assert health.get("version") == "v1.0.3-offline", health
                return
        except (OSError, urllib.error.URLError, json.JSONDecodeError):
            pass
        time.sleep(1)
    raise RuntimeError("readiness timed out")


def stop():
    global process, output
    if process is None:
        return
    outcome = subprocess.run([str(root / "stop.sh")], cwd=root, capture_output=True, text=True, timeout=85)
    assert outcome.returncode == 0, outcome.stdout + outcome.stderr
    assert process.wait(timeout=25) == 0
    process = None
    output.close()
    output = None
    for port in (53513, 55435):
        with socket.socket() as probe:
            probe.settimeout(1)
            assert probe.connect_ex(("127.0.0.1", port)) != 0, port


try:
    assert not (root / ".yunkun-local").exists(), "package contains an installation"
    for relative in ("start.sh", "stop.sh", "bin/yunkun", "bin/yunkun-offline",
                     "runtime/bin/initdb", "runtime/bin/pg_ctl", "runtime/bin/postgres"):
        assert os.access(root / relative, os.X_OK), relative
    checks.append("clean archive and executable permissions")
    start()
    checks.append("bundled PostgreSQL initialized and health ready")
    html = request("/login/").decode()
    assert "<html" in html and "_next/" in html, "embedded frontend missing"
    checks.append("embedded login frontend served")
    assert api("/api/auth/status")["initialized"] is False
    password = secrets.token_urlsafe(28) + "Aa1"
    token = api("/api/auth/init", {"password": password})["token"]
    api("/api/auth/login", {"username": "yunkun", "password": password})["token"]
    assert api("/api/auth/status")["initialized"] is True
    checks.append("new administrator initialized and logged in")
    tasks = api("/api/tasks", token=token)
    assert tasks in ([], None) or isinstance(tasks, dict) and not tasks.get("tasks"), "unexpected customer tasks"
    kb = api("/api/rag/knowledge-bases", {
        "name": "unix-smoke-synthetic", "visibility": "private",
        "chunk_size": 900, "chunk_overlap": 120,
    }, token)
    assert kb.get("id"), kb
    checks.append("synthetic knowledge base written")
    state_path = root / ".yunkun-local" / "installation.json"
    state_hash = hashlib.sha256(state_path.read_bytes()).hexdigest()
    stop()
    assert (root / ".yunkun-local" / "postgres-data" / "PG_VERSION").is_file()
    checks.append("stop.sh stops both listeners and preserves database")
    start()
    assert hashlib.sha256(state_path.read_bytes()).hexdigest() == state_hash
    api("/api/auth/login", {"username": "yunkun", "password": password})["token"]
    retained = api("/api/rag/knowledge-bases", token=token)
    assert "unix-smoke-synthetic" in json.dumps(retained)
    checks.append("restart retains credentials, session and synthetic data")
    stop()
    result = {"ok": True, "checks": checks}
    result_path.write_text(json.dumps(result, indent=2) + "\n")
    print(json.dumps(result, indent=2))
except BaseException as error:
    result_path.write_text(json.dumps({"ok": False, "checks": checks, "error": str(error)}, indent=2) + "\n")
    raise
finally:
    if process is not None:
        try:
            process.terminate()
            process.wait(timeout=30)
        except (ProcessLookupError, subprocess.TimeoutExpired):
            process.kill()
        if output is not None:
            output.close()
