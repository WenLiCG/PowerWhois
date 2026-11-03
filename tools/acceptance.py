#!/usr/bin/env python3
"""Local mock acceptance; requires Linux/WSL, Redis and the three .work binaries.
Run from repo: python3 tools/acceptance.py [--serve]
--serve keeps the fixture backend on 8080 for browser verification on Vite 8090.
All Redis/config data is temporary. No registrar or public lookup is contacted.
"""
import argparse
import contextlib
import csv
import http.server
import io
import json
import os
from pathlib import Path
import secrets
import selectors
import signal
import shutil
import socket
import subprocess
import tempfile
import threading
import time
import urllib.error
import urllib.parse
import urllib.request

ROOT = Path(__file__).resolve().parents[1]


class Fixture(http.server.BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass

    def do_GET(self):
        domain = urllib.parse.parse_qs(urllib.parse.urlsplit(self.path).query).get("domain", [""])[0]
        if domain.startswith("slow"):
            time.sleep(2)
        self.send_response(500 if domain == "broken.com" else 200)
        self.end_headers()
        with contextlib.suppress(BrokenPipeError, ConnectionResetError):
            self.wfile.write(b"MISSING" if domain == "missing.com" else b"EXISTS")


def free_port():
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        return sock.getsockname()[1]


def main():
    args = argparse.ArgumentParser(description=__doc__)
    args.add_argument("--serve", action="store_true")
    args = args.parse_args()
    children = []
    with tempfile.TemporaryDirectory(prefix="powerwhois-acceptance-") as directory:
        scratch = Path(directory)
        for name in ("typonamer", "powerwhois", "powerwhois-mcp"):
            shutil.copy2(ROOT / ".work" / name, scratch / name)
        log = (scratch / "processes.log").open("w")
        fixture = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Fixture)
        threading.Thread(target=fixture.serve_forever, daemon=True).start()
        redis_port, mcp_port = free_port(), free_port()
        backend_port = 8080 if args.serve else free_port()
        token, mcp_token = secrets.token_urlsafe(40), secrets.token_urlsafe(40)
        # These credentials exist only in this isolated, temporary mock service.
        username, password = "acceptance-admin", "acceptance-test-only-password"
        config = {"logLevel": "Error", "retryOnTimeout": False, "rdapRate": 256,
                  "whoisTimeout": 1, "webCheckDomainLimit": 500,
                  "whoisApis": [{"apiName": "Fixture", "apiUrl": f"http://127.0.0.1:{fixture.server_port}/?domain={{domain}}",
                                 "freeText": ["MISSING"], "takenText": ["EXISTS"], "concurrencyLimit": 32}]}
        (scratch / "config.yaml").write_text(json.dumps(config), encoding="utf-8")
        env = {**os.environ, "POWERWHOIS_CONFIG": str(scratch / "config.yaml"),
               "POWERWHOIS_ADMIN_USER": username, "POWERWHOIS_ADMIN_PASSWORD": password,
               "POWERWHOIS_JWT_SECRET": secrets.token_urlsafe(40), "POWERWHOIS_QUERY_TOKEN": token,
               "POWERWHOIS_MCP_TOKEN": mcp_token, "POWERWHOIS_TOKEN_FILE": "", "REDIS_HOST": "127.0.0.1", "REDIS_PORT": str(redis_port),
               "REDIS_DB": "0", "REDIS_PASSWORD": "", "POWERWHOIS_LISTEN": f"127.0.0.1:{backend_port}",
               "POWERWHOIS_URL": f"http://127.0.0.1:{backend_port}"}

        def start(command, **kwargs):
            process = subprocess.Popen(command, cwd=scratch, env=env, stderr=log, **kwargs)
            children.append(process)
            return process

        def request(path, body=None, bearer=token, headers=None, raw=False, method=None):
            req = urllib.request.Request(env["POWERWHOIS_URL"] + path,
                data=None if body is None else json.dumps(body).encode(),
                headers={"Content-Type": "application/json", "Authorization": "Bearer " + bearer, **(headers or {})}, method=method)
            with urllib.request.urlopen(req, timeout=20) as response:
                data = response.read()
                return data if raw else json.loads(data)

        def cli(*arguments):
            result = subprocess.run([str(scratch / "powerwhois"), *arguments], env=env, capture_output=True, timeout=25)
            assert result.returncode == 0, result.stderr.decode()
            return json.loads(result.stdout)

        try:
            start(["redis-server", "--bind", "127.0.0.1", "--port", str(redis_port), "--save", "", "--appendonly", "yes", "--dir", str(scratch)], stdout=log)
            backend = start([str(scratch / "typonamer")], stdout=log)
            for _ in range(100):
                try:
                    request("/api/web/setting")
                    break
                except (urllib.error.URLError, ConnectionError):
                    assert backend.poll() is None, "backend failed; inspect isolated log"
                    time.sleep(.1)
            else:
                raise AssertionError("backend startup timeout")
            domains = ["taken.com", "missing.com", "broken.com", "slow.com"]
            body = {"domains": domains, "mode": "Fixture", "concurrency": 4, "timeout": 1, "retries": 0}
            expected = ["registered", "not_found", "error", "timeout"]
            direct = request("/api/v1/query", body)["items"]
            assert [r["status"] for r in direct] == expected
            from_cli = cli("query", "--mode", "Fixture", "--timeout", "1", "--concurrency", "4", *domains)["items"]
            assert [r["status"] for r in from_cli] == expected
            task = cli("tasks", "create", "--mode", "Fixture", "--timeout", "1", "--concurrency", "4", "--key", "cross-entry", *domains)
            admin = request("/api/login", {"username": username, "password": password})["token"]
            web_task = request("/api/v1/tasks", body, bearer=admin, headers={"Idempotency-Key": "cross-entry"})
            assert web_task["id"] == task["id"]
            mcp = start([str(scratch / "powerwhois-mcp")], stdin=subprocess.PIPE, stdout=subprocess.PIPE, text=True, bufsize=1)
            sequence = 0

            def rpc(method, params):
                nonlocal sequence
                sequence += 1
                mcp.stdin.write(json.dumps({"jsonrpc": "2.0", "id": sequence, "method": method, "params": params}) + "\n")
                mcp.stdin.flush()
                with selectors.DefaultSelector() as selector:
                    selector.register(mcp.stdout, selectors.EVENT_READ)
                    assert selector.select(20), "MCP response timeout"
                message = json.loads(mcp.stdout.readline())
                assert message["id"] == sequence and "error" not in message, message
                return message["result"]

            rpc("initialize", {"protocolVersion": "2025-06-18", "capabilities": {}, "clientInfo": {"name": "acceptance", "version": "1"}})
            mcp.stdin.write(json.dumps({"jsonrpc": "2.0", "method": "notifications/initialized"}) + "\n"); mcp.stdin.flush()
            tools = rpc("tools/list", {})["tools"]
            assert len(tools) == 11 and not any("register" in tool["name"] for tool in tools)
            result = rpc("tools/call", {"name": "create_task", "arguments": {**body, "idempotencyKey": "cross-entry"}})
            assert result["structuredContent"]["id"] == task["id"]
            for _ in range(100):
                status = cli("tasks", "get", "--id", task["id"])
                if status["state"] == "completed": break
                time.sleep(.1)
            else: raise AssertionError("task timeout")
            api_export = request(f"/api/v1/tasks/{task['id']}/export")
            cli_export = cli("tasks", "export", "--id", task["id"])
            mcp_export = rpc("tools/call", {"name": "export_task", "arguments": {"id": task["id"]}})["structuredContent"]
            web_export = request(f"/api/v1/tasks/{task['id']}/export", bearer=admin)
            assert api_export == cli_export == mcp_export == web_export
            assert [r["status"] for r in api_export["items"]] == expected
            csv_data = request(f"/api/v1/tasks/{task['id']}/export?format=csv", raw=True)
            assert len(list(csv.reader(io.StringIO(csv_data.decode())))) == 5
            mcp.stdin.close(); mcp.wait(timeout=5)
            backend.terminate(); backend.wait(timeout=10)
            backend = start([str(scratch / "typonamer")], stdout=log)
            for _ in range(100):
                try:
                    assert request(f"/api/v1/tasks/{task['id']}/export") == api_export
                    break
                except urllib.error.URLError: time.sleep(.1)
            else: raise AssertionError("restart timeout")
            start([str(scratch / "powerwhois-mcp"), "--http", f"127.0.0.1:{mcp_port}"], stdout=log)
            for _ in range(100):
                try:
                    urllib.request.urlopen(f"http://127.0.0.1:{mcp_port}/mcp", timeout=1)
                except urllib.error.HTTPError as error:
                    assert error.code == 401
                    break
                except urllib.error.URLError: time.sleep(.1)
            init = {"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"acceptance","version":"1"}}}
            req = urllib.request.Request(f"http://127.0.0.1:{mcp_port}/mcp", data=json.dumps(init).encode(), headers={"Content-Type":"application/json","Accept":"application/json, text/event-stream","Authorization":"Bearer " + mcp_token})
            with urllib.request.urlopen(req, timeout=10) as response: assert "result" in json.load(response)
            log.flush()
            logs = (scratch / "processes.log").read_text()
            assert not any(secret in logs for secret in [password, token, mcp_token, admin])
            print("PASS: API/CLI/MCP and authenticated Web API share task ID and equivalent JSON exports; HTTP MCP auth, per-domain timeout, no secret logs, service restart.", flush=True)
            if args.serve:
                print(f"Browser fixture ready: backend 127.0.0.1:{backend_port}; task {task['id']}. Use temporary credentials documented in this test. Ctrl+C removes fixture processes/data.", flush=True)
                signal.pause()
        finally:
            for child in reversed(children):
                if child.poll() is None:
                    child.terminate()
                    try: child.wait(timeout=10)
                    except subprocess.TimeoutExpired: child.kill(); child.wait()
            fixture.shutdown(); fixture.server_close(); log.close()


if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        pass
