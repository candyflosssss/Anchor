#!/usr/bin/env python3
"""Isolated Linux management/recovery smoke; resource_guard.sh run is required.

Compiles only a temporary test executable, never a release/package. Optional
Codex execution has a fresh HOME/CODEX_HOME and no inherited credentials. All
model requests use the loopback mock; ambient outbound proxies are disabled or
replaced with a closed loopback endpoint for the optional Codex process.
"""
from __future__ import annotations

import argparse
import json
import os
from pathlib import Path
import signal
import queue
import socket
import subprocess
import tempfile
import threading
import time
import tomllib
from http.server import BaseHTTPRequestHandler, HTTPServer
from urllib.error import URLError
from urllib.request import ProxyHandler, Request, build_opener

ROOT = Path(__file__).resolve().parents[1]
OPENER = build_opener(ProxyHandler({}))


class Mock(BaseHTTPRequestHandler):
    calls = 0
    custom_calls = 0

    def log_message(self, *_args):
        pass

    def do_POST(self):
        assert self.path == "/v1/responses"
        assert self.headers.get("Authorization") == "Bearer mock-provider-key"
        body = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
        assert body["model"] == "mock-actual"
        assert body["store"] is False
        type(self).calls += 1
        if any(t.get("type") == "custom" for t in body.get("tools", [])):
            type(self).custom_calls += 1
        item = {"id": "msg_mock", "type": "message", "role": "assistant", "status": "completed",
                "content": [{"type": "output_text", "text": "ANCHOR_ISOLATED_OK", "annotations": []}]}
        response = {"id": "resp_mock", "object": "response", "created_at": 0, "model": "mock-actual",
                    "status": "completed", "output": [item],
                    "usage": {"input_tokens": 1, "output_tokens": 1, "total_tokens": 2}}
        self.send_response(200)
        if not body.get("stream"):
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(json.dumps(response).encode())
            return
        self.send_header("Content-Type", "text/event-stream")
        self.end_headers()
        events = [
            {"type": "response.created", "response": {**response, "status": "in_progress", "output": []}},
            {"type": "response.output_item.added", "output_index": 0,
             "item": {**item, "status": "in_progress", "content": []}},
            {"type": "response.content_part.added", "output_index": 0, "content_index": 0,
             "item_id": item["id"], "part": {"type": "output_text", "text": "", "annotations": []}},
            {"type": "response.output_text.delta", "output_index": 0, "content_index": 0,
             "item_id": item["id"], "delta": "ANCHOR_ISOLATED_OK"},
            {"type": "response.output_text.done", "output_index": 0, "content_index": 0,
             "item_id": item["id"], "text": "ANCHOR_ISOLATED_OK"},
            {"type": "response.output_item.done", "output_index": 0, "item": item},
            {"type": "response.completed", "response": response},
        ]
        for sequence, event in enumerate(events):
            event["sequence_number"] = sequence
            self.wfile.write(("event: " + event["type"] + "\ndata: " + json.dumps(event) + "\n\n").encode())
            self.wfile.flush()


def addresses():
    sockets = [socket.socket(), socket.socket()]
    try:
        for sock in sockets:
            sock.bind(("127.0.0.1", 0))
        return [f"127.0.0.1:{sock.getsockname()[1]}" for sock in sockets]
    finally:
        for sock in sockets:
            sock.close()



def app_server_smoke(binary, env, work):
    """Exercise the actual desktop SSH backend protocol, not the desktop UI."""
    events = queue.Queue()
    messages = []
    with (work / "app-server.log").open("w") as log:
        server = subprocess.Popen([str(binary), "app-server"], cwd=work, env=env,
                                  stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=log,
                                  text=True, bufsize=1, start_new_session=True)
        def reader():
            for line in server.stdout:
                try:
                    events.put(json.loads(line))
                except json.JSONDecodeError:
                    events.put({"invalid_stdout": line})
            events.put({"eof": True})
        worker = threading.Thread(target=reader, daemon=True)
        worker.start()
        sequence = 0
        def receive(deadline):
            if time.monotonic() >= deadline:
                raise RuntimeError("app-server JSON-RPC timeout")
            try:
                event = events.get(timeout=max(0.01, deadline - time.monotonic()))
            except queue.Empty:
                raise RuntimeError("app-server JSON-RPC timeout") from None
            if "eof" in event or "invalid_stdout" in event:
                raise RuntimeError(f"app-server protocol failure: {event}")
            messages.append(event)
            return event
        def send(method, params, request_id=None):
            message = {"method": method, "params": params}
            if request_id is not None:
                message["id"] = request_id
            server.stdin.write(json.dumps(message) + "\n")
            server.stdin.flush()
        def rpc(method, params):
            nonlocal sequence
            sequence += 1
            send(method, params, sequence)
            deadline = time.monotonic() + 60
            while time.monotonic() < deadline:
                event = receive(deadline)
                if event.get("id") == sequence:
                    if "error" in event:
                        raise RuntimeError(f"{method}: {event['error']}")
                    return event["result"]
            raise RuntimeError(f"{method} timeout")
        try:
            rpc("initialize", {"clientInfo": {"name": "anchor_smoke", "version": "0.1.0"},
                               "capabilities": {"experimentalApi": True}})
            send("initialized", {})
            models = rpc("model/list", {"includeHidden": False})["data"]
            assert {m["model"] for m in models} == {"anchor-default", "anchor-alternative"}, models
            for alias in (None, "anchor-alternative"):
                params = {"cwd": str(work), "approvalPolicy": "never", "sandbox": "read-only"}
                if alias:
                    params["model"] = alias
                result = rpc("thread/start", params)
                assert result["model"] == (alias or "anchor-default"), result
                thread_id = result["thread"]["id"]
                begin = len(messages)
                rpc("turn/start", {"threadId": thread_id, "input": [
                    {"type": "text", "text": "Respond with ANCHOR_ISOLATED_OK. Do not use tools.", "text_elements": []}]})
                deadline = time.monotonic() + 60
                while not any(m.get("method") == "turn/completed" and
                              m.get("params", {}).get("threadId") == thread_id for m in messages[begin:]):
                    receive(deadline)
                completed = next(m for m in messages[begin:] if m.get("method") == "turn/completed")
                assert completed["params"]["turn"]["status"] == "completed", completed
                assert "ANCHOR_ISOLATED_OK" in json.dumps(messages[begin:])
        finally:
            if server.poll() is None:
                os.killpg(server.pid, signal.SIGTERM)
                try:
                    server.wait(timeout=10)
                except subprocess.TimeoutExpired:
                    os.killpg(server.pid, signal.SIGKILL)
                    server.wait(timeout=5)
            server.stdin.close()
            worker.join(timeout=5)
            server.stdout.close()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--codex-binary", type=Path, help="optional installed CLI, tested with isolated HOME")
    parser.add_argument("--codex-catalog", type=Path, default=ROOT / "internal/state/testdata/codex-models.json",
                        help="local source catalog; defaults to synthetic offline fixture")
    parser.add_argument("--codex-source", default="fixture-source")
    parser.add_argument("--codex-version", default="0.159.3")
    args = parser.parse_args()
    if os.name != "posix":
        raise RuntimeError("this native lifecycle smoke requires Linux")
    with tempfile.TemporaryDirectory(prefix="anchor-agent-smoke-") as temporary:
        work = Path(temporary)
        binary, data, home = work / "anchor-test", work / "state", work / "codex-home"
        home.mkdir(mode=0o700)
        config = home / "config.toml"
        config.write_text('model = "old-model"\n# private fixture\nfixture = "keep"\n')
        config.chmod(0o600)
        env = dict(os.environ, GOMAXPROCS="2")
        subprocess.run(["go", "build", "-p", "1", "-o", str(binary), "./cmd/anchor"],
                       cwd=ROOT, env=env, check=True, timeout=120)

        def run(*arguments, stdin=None, raw=False, success=True):
            result = subprocess.run([str(binary), "--data-dir", str(data), *arguments],
                                    input=stdin, text=True, capture_output=True, timeout=15)
            if success and result.returncode:
                raise RuntimeError(f"command {arguments[0:2]} failed: {result.stderr}")
            if not success:
                assert result.returncode != 0
                return result
            return result.stdout.strip() if raw else json.loads(result.stdout)

        control, gateway = addresses()
        run("init", "--control-address", control, "--gateway-address", gateway)
        service = None
        logs = work / "service.log"
        mock = HTTPServer(("127.0.0.1", 0), Mock)
        worker = threading.Thread(target=mock.serve_forever)
        worker.start()
        log = logs.open("w")
        try:
            def start():
                nonlocal service
                service = subprocess.Popen([str(binary), "--data-dir", str(data), "serve"],
                                           stdout=subprocess.DEVNULL, stderr=log, start_new_session=True)
                owner = (data / "owner.token").read_text().strip()
                until = time.monotonic() + 10
                while time.monotonic() < until:
                    if service.poll() is not None:
                        raise RuntimeError("service exited before ready")
                    try:
                        request = Request(f"http://{control}/control/v1/status",
                                          headers={"Authorization": f"Bearer {owner}"})
                        with OPENER.open(request, timeout=1) as response:
                            status = json.load(response)
                        # Listener readiness alone is not binding readiness.
                        if not status["bindings"] or status["bindings"][0]["status"] == "applied":
                            return
                    except (URLError, TimeoutError):
                        pass
                    time.sleep(0.05)
                raise RuntimeError("readiness timeout")

            def stop(signum=signal.SIGTERM):
                nonlocal service
                os.killpg(service.pid, signum)
                service.wait(timeout=15)
                if signum == signal.SIGTERM:
                    assert service.returncode == 0
                service = None

            start()
            run("context", "add", "local", "--local")
            run("context", "use", "local")
            run("provider", "add", "--id", "mock", "--base-url",
                f"http://127.0.0.1:{mock.server_port}/v1", "--secret-stdin", stdin="mock-provider-key")
            for alias in ("anchor-default", "anchor-alternative"):
                run("model", "add", "--id", alias, "--candidate", "mock:mock-actual", "--tools", "--streaming",
                    "--codex-catalog", str(args.codex_catalog), "--codex-model", args.codex_source,
                    "--codex-version", args.codex_version)
            binding = run("agent", "add", "--id", "isolated", "--codex-home", str(home),
                          "--model", "anchor-default", "--anchor-executable", str(binary), "--approve")
            assert binding["revision"] == 1
            assert tomllib.loads(config.read_text())["model"] == "old-model"  # register != apply
            plan = run("agent", "plan", "isolated")
            run("agent", "apply", "isolated", "--revision", "1", "--fingerprint", plan["fingerprint"])
            doc = tomllib.loads(config.read_text())
            catalog = Path(doc["model_catalog_json"])
            entries = json.loads(catalog.read_text())["models"]
            assert {m["slug"] for m in entries} == {"anchor-default", "anchor-alternative"}
            assert all(m["upgrade"] is None and not m["prefer_websockets"] for m in entries)
            assert binding["model_scope"] == "all-compatible"
            provider = doc["model_providers"][doc["model_provider"]]
            credential = provider["auth"]
            token = subprocess.check_output([credential["command"], *credential["args"]], text=True).strip()
            assert token and token not in config.read_text()
            request = Request(f"http://{gateway}/v1/responses", method="POST",
                              data=json.dumps({"model": "anchor-default", "stream": True,
                                               "tools": [{"type": "custom", "name": "patch"}],
                                               "input": [{"type": "custom_tool_call_output", "call_id": "mock", "output": "done"}]}).encode(),
                              headers={"Authorization": f"Bearer {token}", "Content-Type": "application/json"})
            with OPENER.open(request, timeout=5) as response:
                events = response.read().decode()
                assert "response.completed" in events and "ANCHOR_ISOLATED_OK" in events
            # Default is not an authorization boundary; both aliases route to the mock.
            request.data = json.dumps({"model": "anchor-alternative", "input": "mock"}).encode()
            with OPENER.open(request, timeout=5) as response:
                assert json.load(response)["status"] == "completed"
            request = Request(f"http://{gateway}/v1/models", headers={"Authorization": f"Bearer {token}"})
            with OPENER.open(request, timeout=5) as response:
                assert {m["id"] for m in json.load(response)["data"]} == {"anchor-default", "anchor-alternative"}
            run("recover", success=False)  # running instance owns the lock
            codex_version = None
            if args.codex_binary:
                isolated_home = work / "empty-home"
                isolated_home.mkdir(mode=0o700)
                codex_env = {"PATH": os.environ["PATH"], "HOME": str(isolated_home),
                             "CODEX_HOME": str(home), "XDG_CONFIG_HOME": str(isolated_home),
                             "TMPDIR": str(work), "LANG": "C.UTF-8", "TERM": "dumb",
                             "HTTP_PROXY": "http://127.0.0.1:9", "HTTPS_PROXY": "http://127.0.0.1:9",
                             "ALL_PROXY": "http://127.0.0.1:9", "NO_PROXY": "127.0.0.1,localhost,::1"}
                codex_version = subprocess.check_output([str(args.codex_binary), "--version"],
                                                        env=codex_env, text=True, timeout=15).strip()
                assert codex_version.split()[-1] == args.codex_version, "metadata version must match tested binary"
                for override in ([], ["--model", "anchor-alternative"]):
                    result = subprocess.run([str(args.codex_binary), "exec", "--skip-git-repo-check",
                                             "--sandbox", "read-only", *override,
                                             "Respond with ANCHOR_ISOLATED_OK. Do not use tools."],
                                            cwd=work, env=codex_env, capture_output=True, text=True, timeout=60)
                    if result.returncode or "ANCHOR_ISOLATED_OK" not in result.stdout:
                        raise RuntimeError(f"isolated Codex smoke failed ({result.returncode}): {result.stderr[-5000:]}")
                app_server_smoke(args.codex_binary, codex_env, work)
            changed = run("agent", "set-models", "isolated", "--revision", "1", "--model", "anchor-alternative",
                          "--model-scope", "single")
            assert changed["binding"]["revision"] == 2 and changed["apply_required"]
            assert tomllib.loads(config.read_text())["model"] == "old-model" and not catalog.exists()
            assert run("agent", "token", "isolated", raw=True) == token
            with OPENER.open(request, timeout=5) as response:
                assert {m["id"] for m in json.load(response)["data"]} == {"anchor-alternative"}
            changed = run("agent", "set-models", "isolated", "--revision", "2", "--model", "anchor-default",
                          "--model-scope", "all-compatible")
            assert changed["binding"]["revision"] == 3
            refreshed = run("agent", "plan", "isolated")
            run("agent", "apply", "isolated", "--revision", "3", "--fingerprint", refreshed["fingerprint"])
            stop()
            restored = tomllib.loads(config.read_text())
            assert restored["model"] == "old-model" and "model_provider" not in restored
            assert restored["fixture"] == "keep" and "model_providers" not in restored
            assert "model_catalog_json" not in restored and not catalog.exists()
            start()  # approved enabled binding reapplies
            assert tomllib.loads(config.read_text())["model"] == "anchor-default"
            stop(signal.SIGKILL)
            recovery = run("recover", "--id", "isolated")
            assert recovery[0]["status"] == "restored"
            assert tomllib.loads(config.read_text())["model"] == "old-model"
            assert not catalog.exists()
            start()
            config.write_text(config.read_text().replace('model = "anchor-default"', 'model = "user-edit"', 1))
            stop()
            assert tomllib.loads(config.read_text())["model"] == "user-edit"
            assert run("recover", "--id", "isolated", success=False).returncode == 1
            log.flush()
            assert token not in logs.read_text() and "mock-provider-key" not in logs.read_text()
            print(json.dumps({"agent_smoke": "passed", "lifecycle": "apply/normal-stop/restart/crash/independent-recovery/conflict",
                              "mock_requests": Mock.calls, "custom_tool_requests": Mock.custom_calls,
                              "codex_version": codex_version, "catalog_models": 2,
                              "app_server": "model/list and default/override turns passed" if args.codex_binary else "not_run",
                              "real_upstream": "not_used"}))
        finally:
            if service and service.poll() is None:
                os.killpg(service.pid, signal.SIGTERM)
                try:
                    service.wait(timeout=15)
                except subprocess.TimeoutExpired:
                    os.killpg(service.pid, signal.SIGKILL)
                    service.wait(timeout=5)
            mock.shutdown()
            mock.server_close()
            worker.join(timeout=5)
            log.close()


if __name__ == "__main__":
    main()
