#!/usr/bin/env python3
"""Exercise native subagent sessions and QuickJS workflows through a local model."""
import http.server
import json
import os
import pathlib
import subprocess
import sys
import tempfile
import threading


class ModelServer(http.server.BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"
    stage = 0
    failure = ""
    children = 0
    structured_retries = 0
    lock = threading.Lock()

    def log_message(self, *_):
        pass

    def do_POST(self):
        body = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
        cls = type(self)
        messages = body["messages"]
        child = any("<active_agent name=" in str(m.get("content", "")) for m in messages if m["role"] in ("system", "developer"))
        offered = {t["function"]["name"] for t in body.get("tools", [])}
        if child:
            with cls.lock:
                if not any(m["role"] == "assistant" for m in messages):
                    cls.children += 1
            if "Agent" in offered:
                cls.failure = "child inherited recursive delegation without an allowlist"
            if "StructuredOutput" in offered:
                if any(m["role"] == "tool" for m in messages):
                    with cls.lock:
                        cls.structured_retries += 1
                    delta, reason = self.tool("StructuredOutput", {"answer": "STRUCTURED_CHILD_OK"})
                else:
                    delta, reason = self.tool("StructuredOutput", {"answer": {"invalid": True}})
            else:
                delta, reason = {"role": "assistant", "content": "CHILD_SESSION_OK"}, "stop"
        else:
            if not {"Agent", "get_subagent_result", "steer_subagent", "SubagentWorkflow"} <= offered:
                cls.failure = "built-in subagent tools are missing: " + repr(offered)
            with cls.lock:
                stage = cls.stage
                cls.stage += 1
            results = [m for m in messages if m["role"] == "tool"]
            last = results[-1].get("content", "") if results else ""
            if stage == 0:
                delta, reason = self.tool("subagent_rpc_proof", {})
            elif stage == 1:
                if "RPC_BRIDGE_OK" not in str(last):
                    cls.failure = "cross-extension RPC failed: " + str(last)
                delta, reason = self.tool("Agent", {"prompt": "Return CHILD_SESSION_OK", "description": "Native foreground proof", "subagent_type": "general-purpose", "run_in_background": False})
            elif stage == 2:
                if "CHILD_SESSION_OK" not in str(last):
                    cls.failure = "foreground result missing: " + str(last)
                delta, reason = self.tool("SubagentWorkflow", {"script": "export const meta={name:'smoke',description:'Native workflow proof'}; const result=await pipeline(args,item=>agent(item,{schema:{type:'object',properties:{answer:{type:'string'}},required:['answer']}})); return result;", "args": ["one", "two"]})
            elif stage == 3:
                import re
                match = re.search(r"wf_[a-z0-9]+", str(last))
                if not match:
                    cls.failure = "workflow did not return run id: " + str(last)
                    delta, reason = {"role": "assistant", "content": "SUBAGENTS_FAILED"}, "stop"
                else:
                    delta, reason = self.tool("get_subagent_result", {"agent_id": match[0], "wait": True})
            else:
                if stage == 4 and "STRUCTURED_CHILD_OK" not in str(last):
                    cls.failure = "structured workflow result missing: " + str(last)
                delta, reason = {"role": "assistant", "content": "SUBAGENTS_FAILED" if cls.failure else "SUBAGENT_RUNTIME_OK"}, "stop"
        chunks = [{"id": "subagent-smoke", "object": "chat.completion.chunk", "created": 0, "model": "subagent-smoke", "choices": [{"index": 0, "delta": delta, "finish_reason": None}]}, {"id": "subagent-smoke", "choices": [{"index": 0, "delta": {}, "finish_reason": reason}], "usage": {"prompt_tokens": 20, "completion_tokens": 5}}]
        wire = "".join("data: " + json.dumps(c) + "\n\n" for c in chunks) + "data: [DONE]\n\n"
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.send_header("Content-Length", str(len(wire.encode())))
        self.end_headers()
        self.wfile.write(wire.encode())

    def tool(self, name, args):
        return {"role": "assistant", "tool_calls": [{"index": 0, "id": "call_" + name + "_" + str(type(self).stage), "type": "function", "function": {"name": name, "arguments": json.dumps(args)}}]}, "tool_calls"


def main():
    binary = pathlib.Path(sys.argv[1]).resolve()
    with tempfile.TemporaryDirectory(prefix="ompig-subagents-smoke-") as directory:
        root = pathlib.Path(directory)
        agent_dir = root / "agent"
        agent_dir.mkdir()
        (root / ".pi").mkdir()
        (root / ".pi" / "subagents.json").write_text(json.dumps({"enabled": True}))
        extension = root / "rpc-proof.ts"
        extension.write_text("""import * as sdk from "@earendil-works/pi-coding-agent";
        export default function(pi) {
          let roleOnlyContext = false;
          pi.on("session_start", (_event, ctx) => { roleOnlyContext = !("scopedModels" in ctx); });
          pi.registerTool({name:'subagent_rpc_proof',label:'RPC proof',description:'Verify built-in RPC bridge',parameters:{type:'object',properties:{}},
            async execute() {
              if (!roleOnlyContext || typeof sdk.resolveCliModel !== 'function' ||
                  'resolveModelScopeWithDiagnostics' in sdk ||
                  typeof sdk.SettingsManager.prototype.getEnabledModels === 'function' ||
                  typeof sdk.AgentSession.prototype.setScopedModels === 'function') {
                throw new Error('obsolete scoped-model SDK surface remains');
              }
              const result = await new Promise((resolve,reject)=> {
                const off=pi.events.on('subagents:rpc:ping:reply:smoke', reply=> {
                  clearTimeout(timer); off(); resolve(reply);
                });
                const timer=setTimeout(()=>{off(); reject(new Error('RPC timeout'));},5000);
                pi.events.emit('subagents:rpc:ping',{requestId:'smoke'});
              });
              if (!result.success || result.data.version!==2) throw new Error(JSON.stringify(result));
              return {content:[{type:'text',text:'RPC_BRIDGE_OK'}],details:{}};
            }
          });
        }""")
        server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), ModelServer)
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        try:
            (agent_dir / "models.json").write_text(json.dumps({"providers": {"subagent-smoke": {"api": "openai-completions", "baseUrl": f"http://127.0.0.1:{server.server_port}/v1", "apiKey": "local-fixture-only", "models": [{"id": "subagent-smoke"}]}}}))
            (agent_dir / "settings.json").write_text(json.dumps({"quietStartup": True, "enableInstallTelemetry": False, "extensionsAutoUpgrade": False}))
            env = {k: v for k, v in os.environ.items() if not k.endswith(("_API_KEY", "_AUTH_TOKEN", "_OAUTH_TOKEN"))}
            env.update(HOME=str(root), PIG_HOME=str(root / "library"), OMPIG_SLIM_CODING_AGENT_DIR=str(agent_dir), PI_OFFLINE="1", PI_TELEMETRY="0", PI_SKIP_VERSION_CHECK="1")
            result = subprocess.run([str(binary), "--provider", "subagent-smoke", "--model", "subagent-smoke", "--extension", str(extension), "--print", "--offline", "--no-context-files", "--no-skills", "--no-prompt-templates", "--no-themes", "Exercise native subagents."], cwd=root, env=env, stdin=subprocess.DEVNULL, capture_output=True, text=True, timeout=60)
            if result.returncode or ModelServer.failure or "SUBAGENT_RUNTIME_OK" not in result.stdout or ModelServer.children != 3 or ModelServer.structured_retries != 2:
                raise RuntimeError(f"exit={result.returncode}, children={ModelServer.children}: {ModelServer.failure}\nstdout:\n{result.stdout}\nstderr:\n{result.stderr}")
            sessions = list(agent_dir.glob("sessions/**/subagents/**/*.jsonl"))
            if len(sessions) != 3:
                raise RuntimeError(f"expected three persisted child sessions, found {sessions}")
            print("extension RPC, native foreground child, isolated delegation, schema correction, QuickJS pipeline, and persisted sessions: OK")
        finally:
            server.shutdown()
            server.server_close()
            thread.join()


if __name__ == "__main__":
    main()
