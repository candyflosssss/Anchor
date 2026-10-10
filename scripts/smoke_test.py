#!/usr/bin/env python3
"""Linux/Windows packaged-binary smoke test; run through resource_guard.sh run.

Owns only its TemporaryDirectory, local mock HTTP server and process group.
Never contacts a real provider or reads the user's existing Anchor/Agent state.
"""
from __future__ import annotations

import argparse
import json
import os
from pathlib import Path
import signal
import socket
import subprocess
import tempfile
import threading
import time
from http.server import BaseHTTPRequestHandler, HTTPServer
from urllib.error import URLError
from urllib.request import ProxyHandler, Request, build_opener

ROOT = Path(__file__).resolve().parents[1]
OPENER = build_opener(ProxyHandler({}))
COMMAND = ["go", "run", "./cmd/anchor"]


def run(data: Path, *args: str, stdin: str = "") -> object:
    result = subprocess.run(
        [*COMMAND, "--data-dir", str(data), *args],
        cwd=ROOT, input=stdin, text=True, encoding="utf-8", capture_output=True, timeout=60,
    )
    if result.returncode:
        raise RuntimeError(f"CLI command failed: {args[0]} (diagnostics retained only in task memory)")
    return json.loads(result.stdout)


def free_addresses() -> tuple[str, str]:
    sockets = [socket.socket() for _ in range(2)]
    try:
        for sock in sockets:
            sock.bind(("127.0.0.1", 0))
        return tuple(f"127.0.0.1:{sock.getsockname()[1]}" for sock in sockets)
    finally:
        for sock in sockets:
            sock.close()


class MockProvider(BaseHTTPRequestHandler):
    def log_message(self, *_: object) -> None:
        pass

    def do_POST(self) -> None:
        assert self.path == "/v1/responses"
        assert self.headers.get("Authorization") == "Bearer smoke-provider-key"
        body = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
        assert body["model"] == "upstream-smoke" and body["store"] is False
        response = json.dumps({"id": "smoke-response", "output": [], "usage": {"input_tokens": 1}}).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(response)))
        self.end_headers()
        self.wfile.write(response)


def main() -> None:
    global COMMAND
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", type=Path, help="test an extracted native executable instead of go run")
    parser.add_argument("--expected-version", default="0.1.0-dev")
    args = parser.parse_args()
    if args.binary:
        COMMAND = [str(args.binary.resolve(strict=True))]
    if os.name == "nt" and not args.binary:
        raise RuntimeError("Windows acceptance requires the actual packaged executable")
    if os.name != "nt" and not Path("/proc").is_dir():
        raise RuntimeError("this smoke test requires Linux or Windows")
    os.environ.setdefault("GOMAXPROCS", "2")
    os.environ["GOFLAGS"] = os.environ.get("GOFLAGS", "") + " -p=1"
    with tempfile.TemporaryDirectory(prefix="anchor-smoke-") as temp:
        data = Path(temp) / "state"
        assert run(data, "version") == {"version": args.expected_version}
        control, gateway = free_addresses()
        initialized = run(data, "init", "--control-address", control, "--gateway-address", gateway)
        assert initialized == run(data, "init")  # Stable identity and settings.
        owner = (data / "owner.token").read_text().strip()
        if os.name == "nt":
            # Inspect only our isolated state, never the user's real configuration.
            acl = subprocess.run(["powershell.exe", "-NoProfile", "-NonInteractive", "-Command",
                "$ErrorActionPreference='Stop'; $sid=[System.Security.Principal.WindowsIdentity]::GetCurrent().User; "
                "foreach($p in @($env:ANCHOR_SMOKE_STATE, ([IO.Path]::Combine($env:ANCHOR_SMOKE_STATE,'owner.token')), "
                "([IO.Path]::Combine($env:ANCHOR_SMOKE_STATE,'state.db')))) { "
                "if([IO.Directory]::Exists($p)){$a=[IO.Directory]::GetAccessControl($p)}"
                "else{$a=[IO.File]::GetAccessControl($p)}; "
                "if(-not $a.AreAccessRulesProtected){throw 'unprotected ACL'}; "
                "$rules=$a.GetAccessRules($true,$true,[System.Security.Principal.SecurityIdentifier]); "
                "if($rules.Count -lt 1){throw 'missing ACL'}; foreach($r in $rules){ "
                "if($r.IdentityReference.Value -ne $sid.Value -or $r.AccessControlType -ne 'Allow'){throw 'unexpected ACL'}}}"],
                env={**os.environ, "ANCHOR_SMOKE_STATE": str(data)}, capture_output=True, timeout=30)
            if acl.returncode:
                raise RuntimeError("isolated Windows state ACL verification failed: " + acl.stderr.decode("utf-8", errors="replace"))
        service = None
        mock = None
        worker = None
        try:
            service = subprocess.Popen(
                [*COMMAND, "--data-dir", str(data), "serve"],
                cwd=ROOT, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                text=True, encoding="utf-8",
                **({"creationflags": subprocess.CREATE_NEW_PROCESS_GROUP} if os.name == "nt"
                   else {"start_new_session": True}),
            )
            mock = HTTPServer(("127.0.0.1", 0), MockProvider)
            worker = threading.Thread(target=mock.serve_forever)
            worker.start()
            until = time.monotonic() + 30
            while True:
                if service.poll() is not None:
                    raise RuntimeError("core exited before readiness")
                try:
                    request = Request(f"http://{control}/control/v1/status", headers={"Authorization": f"Bearer {owner}"})
                    with OPENER.open(request, timeout=1) as response:
                        assert json.load(response)["core"] == "ready"
                    break
                except (URLError, TimeoutError):
                    if time.monotonic() > until:
                        raise RuntimeError("readiness deadline exceeded")
                    time.sleep(0.05)
            duplicate = subprocess.run([*COMMAND, "--data-dir", str(data), "serve"],
                                       capture_output=True, timeout=10)
            assert duplicate.returncode != 0 and b"instance already in use" in duplicate.stderr
            run(data, "context", "add", "local", "--local")
            run(data, "context", "use", "local")
            assert run(data, "status")["core"] == "ready"
            saved = run(data, "provider", "add", "--id", "摸鱼AI", "--base-url",
                        f"http://127.0.0.1:{mock.server_port}/v1", "--secret-stdin", stdin="smoke-provider-key")
            assert saved["has_secret"] and "smoke-provider-key" not in json.dumps(saved)
            run(data, "model", "add", "--id", "public-smoke", "--candidate", "摸鱼AI:upstream-smoke")
            grant = run(data, "access", "grant", "--label", "smoke", "--scope", "gateway", "--show-once")
            request = Request(f"http://{gateway}/v1/responses", method="POST",
                              data=json.dumps({"model": "public-smoke", "input": "test"}).encode(),
                              headers={"Authorization": f"Bearer {grant['token']}", "Content-Type": "application/json"})
            with OPENER.open(request, timeout=5) as response:
                assert json.load(response)["id"] == "smoke-response"
            run(data, "access", "revoke", grant["access"]["id"])
            assert run(data, "status")["core"] == "ready"  # CLI exits did not stop the core.
            if args.binary:
                application = service.pid
            else:
                # Go may spawn the program from a non-main OS thread.
                children = {
                    int(pid)
                    for file in Path(f"/proc/{service.pid}/task").glob("*/children")
                    for pid in file.read_text().split()
                }
                applications = [pid for pid in children if Path(f"/proc/{pid}/exe").resolve().name == "anchor"]
                assert len(applications) == 1, "expected one owned go-run application child"
                application = applications[0]
            if os.name == "nt":
                service.send_signal(signal.CTRL_BREAK_EVENT)
            else:
                os.kill(application, signal.SIGTERM)
            stdout, stderr = service.communicate(timeout=15)
            assert service.returncode == 0 and stdout == "" and '"msg":"core stopped"' in stderr
            assert "smoke-provider-key" not in stderr and grant["token"] not in stderr
            assert run(data, "init") == initialized  # Reopen persisted state after graceful shutdown.
            for address in (control, gateway):
                host, port = address.rsplit(":", 1)
                with socket.socket() as sock:
                    assert sock.connect_ex((host, int(port))) != 0  # Both owned listeners were released.
        finally:
            if mock is not None:
                if worker is not None and worker.is_alive():
                    mock.shutdown()
                    worker.join(timeout=5)
                mock.server_close()
            if service is not None and service.poll() is None:
                if os.name == "nt":
                    service.terminate()
                else:
                    os.killpg(service.pid, signal.SIGTERM)
                try:
                    service.communicate(timeout=15)
                except subprocess.TimeoutExpired:
                    if os.name == "nt":
                        service.kill()
                    else:
                        os.killpg(service.pid, signal.SIGKILL)
                    service.communicate(timeout=5)
        print(json.dumps({"native_cli_smoke": "passed", "mock_gateway": "passed", "graceful_shutdown": "passed", "instance_lock": "passed",
                          "unicode_provider": "passed", "state_reopen": "passed",
                          "windows_acl": "passed" if os.name == "nt" else "not_applicable"}))


if __name__ == "__main__":
    main()
