#!/usr/bin/env python3
# SPDX-License-Identifier: MIT
# Copyright (c) 2025 The OpenConductor Authors.
"""Scripted OpenAI-compatible chat endpoint for scripts/e2e.sh.

A user message containing "CMD: <command>" makes the model call OpenCode's
bash tool with that command; "TOOL: <name> <json args>" calls any tool. Tests
use this to trigger permission dialogs deterministically. Once the tool result comes back, or for any other message
(including title generation), the model replies with plain text.

Non-streaming requests come from openconductor's L2 classifiers: they get a
valid classification after MOCK_DELAY seconds.

Usage: e2e-mock-llm.py PORT
"""
import json
import os
import sys
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


def chunk(delta, finish=None):
    return {
        "id": "mock",
        "object": "chat.completion.chunk",
        "created": 0,
        "model": "mock",
        "choices": [{"index": 0, "delta": delta, "finish_reason": finish}],
    }


def text_of(msg):
    c = msg.get("content")
    if isinstance(c, list):
        return " ".join(p.get("text", "") for p in c if isinstance(p, dict))
    return c or ""


def reply(messages):
    last = messages[-1] if messages else {}
    text = text_of(last) if last.get("role") == "user" else ""
    call = None
    if "CMD:" in text:
        cmd = text.split("CMD:", 1)[1].strip()
        call = ("bash", json.dumps({"command": cmd, "description": "Run the requested command"}))
    elif "TOOL:" in text:
        name, _, args = text.split("TOOL:", 1)[1].strip().partition(" ")
        call = (name, args.strip() or "{}")
    if call:
        call = {"index": 0, "id": "call_1", "type": "function",
                "function": {"name": call[0], "arguments": call[1]}}
        return [chunk({"role": "assistant", "tool_calls": [call]}), chunk({}, "tool_calls")]
    answer = "mock reply: done" if last.get("role") == "tool" else "mock reply"
    return [chunk({"role": "assistant", "content": answer}), chunk({}, "stop")]


class Handler(BaseHTTPRequestHandler):
    def do_POST(self):
        body = json.loads(self.rfile.read(int(self.headers.get("Content-Length", 0))) or b"{}")
        chunks = reply(body.get("messages", []))
        chunks[-1]["usage"] = {"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2}
        if not body.get("stream"):
            time.sleep(float(os.environ.get("MOCK_DELAY", "0")))
            prompt = text_of(body.get("messages", [{}])[-1])
            if "permission request" in prompt:
                content = '{"category":"file_read","description":"mock","confidence":0.9}'
            else:
                content = "WAITING_INPUT"
            msg = {"role": "assistant", "content": content}
            out = {"id": "mock", "object": "chat.completion", "created": 0, "model": "mock",
                   "choices": [{"index": 0, "message": msg, "finish_reason": "stop"}],
                   "usage": chunks[-1]["usage"]}
            data = json.dumps(out).encode()
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(data)))
            self.end_headers()
            self.wfile.write(data)
            return
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.end_headers()
        for c in chunks:
            self.wfile.write(b"data: " + json.dumps(c).encode() + b"\n\n")
        self.wfile.write(b"data: [DONE]\n\n")

    def log_message(self, *args):
        pass


ThreadingHTTPServer(("127.0.0.1", int(sys.argv[1])), Handler).serve_forever()
