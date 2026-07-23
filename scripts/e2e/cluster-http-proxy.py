#!/usr/bin/env python3

import argparse
import http.client
import json
import os
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from urllib.parse import urlsplit


HOP_BY_HOP_HEADERS = {
    "connection",
    "keep-alive",
    "proxy-authenticate",
    "proxy-authorization",
    "te",
    "trailer",
    "transfer-encoding",
    "upgrade",
}


class ProxyState:
    def __init__(self, upstreams, capture_file, upstream_log):
        self.upstreams = [urlsplit(value.rstrip("/")) for value in upstreams]
        self.capture_file = Path(capture_file) if capture_file else None
        self.upstream_log = Path(upstream_log) if upstream_log else None
        self.lock = threading.Lock()
        self.next_upstream = 0

    def choose_upstream(self):
        with self.lock:
            upstream = self.upstreams[self.next_upstream % len(self.upstreams)]
            self.next_upstream += 1
            return upstream

    def append_json(self, path, value):
        if path is None:
            return
        path.parent.mkdir(parents=True, exist_ok=True)
        with self.lock:
            file_descriptor = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_APPEND, 0o600)
            os.chmod(path, 0o600)
            with os.fdopen(file_descriptor, "a", encoding="utf-8") as output:
                output.write(json.dumps(value, separators=(",", ":"), ensure_ascii=True) + "\n")


class ProxyHandler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def do_GET(self):
        self.proxy()

    def do_HEAD(self):
        self.proxy()

    def do_POST(self):
        self.proxy()

    def do_PUT(self):
        self.proxy()

    def do_PATCH(self):
        self.proxy()

    def do_DELETE(self):
        self.proxy()

    def do_OPTIONS(self):
        self.proxy()

    def proxy(self):
        content_length = int(self.headers.get("Content-Length", "0"))
        body = self.rfile.read(content_length) if content_length else b""
        upstream = self.server.state.choose_upstream()
        upstream_label = f"{upstream.scheme}://{upstream.netloc}"
        target_path = f"{upstream.path.rstrip('/')}{self.path}" or "/"
        headers = {
            name: value
            for name, value in self.headers.items()
            if name.lower() not in HOP_BY_HOP_HEADERS | {"host", "content-length"}
        }
        headers["Host"] = upstream.netloc
        if body:
            headers["Content-Length"] = str(len(body))

        capture_enrollment = self.path.split("?", 1)[0] in {
            "/api/open/cluster/v1/challenges",
            "/api/open/cluster/v1/join",
        }
        if capture_enrollment:
            self.server.state.append_json(
                self.server.state.capture_file,
                {
                    "direction": "request",
                    "method": self.command,
                    "path": self.path,
                    "body": body.decode("utf-8", errors="replace"),
                },
            )
        self.server.state.append_json(
            self.server.state.upstream_log,
            {"method": self.command, "path": self.path, "upstream": upstream_label},
        )

        connection_class = http.client.HTTPSConnection if upstream.scheme == "https" else http.client.HTTPConnection
        connection = connection_class(upstream.hostname, upstream.port, timeout=300)
        try:
            connection.request(self.command, target_path, body=body or None, headers=headers)
            response = connection.getresponse()
            response_body = response.read()
            if capture_enrollment:
                self.server.state.append_json(
                    self.server.state.capture_file,
                    {
                        "direction": "response",
                        "method": self.command,
                        "path": self.path,
                        "status": response.status,
                        "body": response_body.decode("utf-8", errors="replace"),
                    },
                )
            self.send_response(response.status)
            for name, value in response.getheaders():
                if name.lower() not in HOP_BY_HOP_HEADERS | {"content-length"}:
                    self.send_header(name, value)
            self.send_header("Content-Length", str(len(response_body)))
            self.end_headers()
            if self.command != "HEAD":
                self.wfile.write(response_body)
        except (BrokenPipeError, ConnectionResetError):
            return
        finally:
            connection.close()

    def log_message(self, *_args):
        return


def parse_args():
    parser = argparse.ArgumentParser(description="Round-robin HTTP proxy for isolated cluster E2E tests")
    parser.add_argument("--listen", required=True, help="host:port to listen on")
    parser.add_argument("--upstream", action="append", required=True, help="upstream base URL; repeat for round robin")
    parser.add_argument("--capture-file", help="optional JSONL request-body capture")
    parser.add_argument("--upstream-log", help="optional JSONL selected-upstream log")
    return parser.parse_args()


def main():
    args = parse_args()
    host, port = args.listen.rsplit(":", 1)
    server = ThreadingHTTPServer((host, int(port)), ProxyHandler)
    server.state = ProxyState(args.upstream, args.capture_file, args.upstream_log)
    server.serve_forever()


if __name__ == "__main__":
    main()
