"""Native Linux upgrade/rollback acceptance; synthetic data and loopback only.

Usage: python3 offline-oneclick-unix-smoke.py CLEAN_INSTALL CLEAN_CANDIDATE RESULT
Both package directories must be disposable, data-free copies. This directly
exercises the trusted supervisor, not GitHub download or a published update.
"""

import hashlib
import json
import os
from pathlib import Path
import platform
import secrets
import shutil
import signal
import socket
import subprocess
import sys
import time
import urllib.error
import urllib.request


def main():
    if platform.system() != "Linux" or os.geteuid() == 0:
        raise RuntimeError("Run on native Linux as a regular user, without sudo")
    root, candidate, result_path = (Path(p).resolve() for p in sys.argv[1:4])
    if root == candidate or root in candidate.parents or candidate in root.parents:
        raise RuntimeError("Use separate disposable package directories")
    for folder in (root, candidate):
        if (folder / ".yunkun-local").exists():
            raise RuntimeError("Refusing an existing installation with private data")
    port, db_port = 53513, 55435
    for number in (port, db_port):
        with socket.socket() as sock:
            sock.bind(("127.0.0.1", number))
    private = root / ".yunkun-local"
    base = f"http://127.0.0.1:{port}"
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
    original = subprocess.check_output([root / "bin/yunkun", "-version"], text=True).strip()
    target = subprocess.check_output([candidate / "bin/yunkun", "-version"], text=True).strip()
    password = secrets.token_hex(24)
    checks = []

    def api(path, body=None, token=None):
        headers = {}
        if token:
            headers["Authorization"] = "Bearer " + token
        if body is not None:
            headers["Content-Type"] = "application/json"
        request = urllib.request.Request(base + path, headers=headers,
            data=json.dumps(body).encode() if body is not None else None)
        with opener.open(request, timeout=4) as response:
            return json.load(response)

    def wait_for(predicate, seconds=150):
        deadline = time.monotonic() + seconds
        while time.monotonic() < deadline:
            if predicate():
                return
            time.sleep(0.2)
        raise RuntimeError("Acceptance stage timed out")

    def ready():
        try:
            return api("/api/health").get("ready")
        except (OSError, ValueError, urllib.error.URLError):
            return False

    def app_pid():
        launcher = json.loads((private / "launcher.json").read_text())["pid"]
        children = Path(f"/proc/{launcher}/task/{launcher}/children").read_text().split()
        for value in children:
            try:
                if Path(f"/proc/{value}/exe").readlink() == root / "bin/yunkun":
                    return int(value)
            except OSError:
                pass
        raise RuntimeError("Managed console child not found")

    def run_update(name, expected_from, fail=False):
        work = private / "updates" / name
        work.mkdir(parents=True)
        shutil.copytree(candidate, work / "candidate", symlinks=False)
        if fail:
            launcher = work / "candidate/bin/yunkun-offline"
            launcher.write_text("#!/bin/sh\nexit 23\n")
            launcher.chmod(0o700)
        helper = work / "file-helper"
        shutil.copy2(root / "bin/yunkun", helper)
        helper.chmod(0o700)
        parent = app_pid()
        job = dict(root=str(root), work=str(work), parent_pid=parent,
            console_port=port, database_port=db_port, public=False,
            goos="linux", goarch={"x86_64": "amd64", "aarch64": "arm64"}[platform.machine()],
            **{"from": expected_from, "to": target})
        job_file = work / "job.json"
        job_file.write_text(json.dumps(job))
        job_file.chmod(0o600)
        with (work / "worker.log").open("ab") as log:
            process = subprocess.Popen([helper, "-offline-worker", job_file],
                stdout=log, stderr=subprocess.STDOUT, start_new_session=True)
        def supervisor_ready():
            if process.poll() is not None:
                raise RuntimeError(f"Supervisor exited before readiness; inspect {work}/worker.log")
            return (work / "ready").exists()
        wait_for(supervisor_ready, 60)
        # Models the app's graceful exit after the apply endpoint stages a job.
        if Path(f"/proc/{parent}/exe").readlink() != root / "bin/yunkun":
            raise RuntimeError("Refusing to stop an unrelated process")
        os.kill(parent, signal.SIGTERM)
        process.wait(timeout=360)
        state = json.loads((private / "update-state.json").read_text())
        expected = "rolled_back" if fail else "completed"
        if state["phase"] != expected or process.returncode != (1 if fail else 0):
            raise RuntimeError(f"Expected {expected}; inspect {work}/worker.log")
        assert (work / "backup-verified").is_file()
        assert not (work / "backup/.yunkun-local/updates").exists()
        checks.append(expected)

    launcher_log = root.parent / "native-update-launcher.log"
    started = None
    try:
        with launcher_log.open("ab") as log:
            started = subprocess.Popen([root / "bin/yunkun-offline", "--no-browser",
                "--port", str(port), "--database-port", str(db_port)],
                stdout=log, stderr=subprocess.STDOUT, start_new_session=True)
        wait_for(ready)
        token = api("/api/auth/init", {"password": password})["token"]
        api("/api/rag/knowledge-bases", {"name": "upgrade-fixture", "visibility": "private",
            "chunk_size": 900, "chunk_overlap": 120}, token)
        task = api("/api/tasks", {"description": "upgrade-local-history",
            "goal": "Local fixture only", "seed_first_intent": False}, token)
        api(f"/api/tasks/{task['id']}/pause", {}, token)
        pool = api("/api/proxy-pools", {"config": {"name": "upgrade-fixture-proxy",
            "kind": "fixed", "enabled": False, "addresses": ["http://127.0.0.1:9"]},
            "secrets": {"proxy_username": "fixture", "proxy_password": "synthetic"}}, token)
        skill = root / "skills/upgrade-custom/SKILL.md"
        skill.parent.mkdir()
        skill.write_text("User-owned fixture")
        keys = {name: hashlib.sha256((root / "bin" / name).read_bytes()).hexdigest()
            for name in ("jwt.key", "proxy-pool.key")}
        run_update("success", original)
        for failed in (False, True):
            if failed:
                run_update("failed-start", target, True)
            assert api("/api/health")["version"] == target
            api("/api/auth/me", token=token)
            api("/api/auth/login", {"username": "yunkun", "password": password})
            assert "upgrade-local-history" in json.dumps(api("/api/tasks", token=token))
            assert "upgrade-fixture" in json.dumps(api("/api/rag/knowledge-bases", token=token))
            pools = api("/api/proxy-pools", token=token)["pools"]
            assert next(p for p in pools if p["id"] == pool["id"])["has_credentials"]
            assert skill.read_text() == "User-owned fixture"
            for name, digest in keys.items():
                assert hashlib.sha256((root / "bin" / name).read_bytes()).hexdigest() == digest
        checks.extend(["password", "session", "history", "knowledge", "proxy credentials",
            "keys", "custom skills", "backup excludes updates"])
        result_path.parent.mkdir(parents=True, exist_ok=True)
        result_path.write_text(json.dumps(dict(passed=True, checks=checks), indent=2))
        print("PASS: native Linux supervisor upgrade and startup-failure rollback")
    finally:
        if (private / "launcher.json").exists():
            subprocess.run([root / "bin/yunkun-offline", "--stop"], check=True, timeout=100)
        if started:
            started.wait(timeout=100)


if __name__ == "__main__":
    main()
