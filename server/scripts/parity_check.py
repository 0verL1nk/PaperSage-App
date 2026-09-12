"""双跑一致性验收：同一组 API 请求打向 Python(FastAPI) 与 Go(gin) 后端，
对响应做规范化（抹平动态字段）后逐字段 diff。

用法（在仓库根 make parity 或 server/ 下直接跑）：
  python scripts/parity_check.py [--py-port 8000] [--go-port 8001]

前置：make build-server 已产出 server/bin/papersage(.exe)；
backend/ 依赖已 uv sync。S3/CLSI 相关端点不在双跑范围（无本地依赖），
由 Go 单测（fake CLSI/内存存储）与 CI E2E 覆盖。
"""
from __future__ import annotations

import argparse
import json
import os
import re
import signal
import socket
import subprocess
import sys
import time
import urllib.error
import urllib.request

ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
PY_DIR = os.path.join(ROOT, "backend")
GO_BIN = os.path.join(ROOT, "server", "bin", "papersage.exe" if os.name == "nt" else "papersage")


def wait_port(port: int, timeout: float = 30.0) -> None:
    deadline = time.time() + timeout
    while time.time() < deadline:
        with socket.socket() as s:
            s.settimeout(0.5)
            try:
                s.connect(("127.0.0.1", port))
                return
            except OSError:
                time.sleep(0.3)
    raise TimeoutError(f"port {port} not ready")


def start_python(port: int) -> subprocess.Popen:
    # 启动姿势说明（Windows 本地双跑的关键）：
    #  - cwd=ROOT + --app-dir backend：让 Config("alembic.ini") 解析失败，
    #    跳过 lifespan 里同步 alembic 调用（在运行中的事件循环上会挂死）
    #  - PYTHONIOENCODING=utf-8：lifespan 的 print("✓") 在 GBK 控制台会崩
    #  - S3_ENDPOINT 指向不可达端口：被测端点不触达 S3
    env = dict(os.environ)
    env.update({
        "PYTHONIOENCODING": "utf-8",
        "PYTHONPATH": PY_DIR,
        "DATABASE_URL": "sqlite+aiosqlite:///" + PY_DIR.replace("\\", "/") + "/parity_py.db",
        "LATEX_CLSI_ENABLED": "false",
        "EMAIL_PROVIDER": "smtp", "SMTP_HOST": "localhost",
        "S3_ENDPOINT": "http://127.0.0.1:9", "S3_ACCESS_KEY": "x", "S3_SECRET_KEY": "x",
    })
    db = os.path.join(PY_DIR, "parity_py.db")
    if os.path.exists(db):
        os.remove(db)
    return subprocess.Popen(
        [uv_bin(), "run", "--project", PY_DIR, "uvicorn", "app.main:app",
         "--port", str(port), "--app-dir", PY_DIR],
        cwd=ROOT, env=env,
        stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
    )


def uv_bin() -> str:
    return os.environ.get("UV", "uv")


def start_go(port: int) -> subprocess.Popen:
    env = dict(os.environ)
    env.update({
        "DATABASE_URL": "sqlite:///./parity_go.db",
        "PORT": str(port),
        "LATEX_CLSI_ENABLED": "false",
    })
    db = os.path.join(ROOT, "server", "parity_go.db")
    if os.path.exists(db):
        os.remove(db)
    return subprocess.Popen([GO_BIN], cwd=os.path.join(ROOT, "server"), env=env,
                            stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)


# ── 动态字段规范化 ────────────────────────────────────────────
UUID_RE = re.compile(r"[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}")
HEX32_RE = re.compile(r"\b[0-9a-f]{32}\b")
TIME_RE = re.compile(r"\d{4}-\d{2}-\d{2}T[\d:.]+\+00:00")
JWT_RE = re.compile(r"eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}")


def normalize(obj):
    if isinstance(obj, dict):
        return {k: normalize(v) for k, v in obj.items()}
    if isinstance(obj, list):
        return [normalize(v) for v in obj]
    if isinstance(obj, str):
        s = UUID_RE.sub("<UUID>", obj)
        s = JWT_RE.sub("<JWT>", s)
        s = HEX32_RE.sub("<HEX32>", s)
        s = TIME_RE.sub("<TIME>", s)
        return s
    return obj


def call(base: str, method: str, path: str, body=None, form=None, token=None, raw=False):
    url = base + path
    data = None
    headers = {}
    if form is not None:
        data = "&".join(f"{k}={v}" for k, v in form.items()).encode()
        headers["Content-Type"] = "application/x-www-form-urlencoded"
    elif body is not None:
        data = json.dumps(body).encode()
        headers["Content-Type"] = "application/json"
    if token:
        headers["Authorization"] = f"Bearer {token}"
    # 307/308 重定向跟随（保持方法与 body），与 axios/浏览器行为一致；
    # FastAPI 与 gin 的 redirect_slashes 重定向方向相反但自洽，比对最终响应
    for _ in range(5):
        req = urllib.request.Request(url, data=data, headers=headers, method=method)
        try:
            with urllib.request.urlopen(req, timeout=10) as resp:
                payload = resp.read()
                status = resp.status
                hdrs = dict(resp.headers)
                redirect = resp.headers.get("Location") if resp.status in (301, 302, 307, 308) else None
        except urllib.error.HTTPError as e:
            payload = e.read()
            status = e.code
            hdrs = dict(e.headers)
            redirect = e.headers.get("Location") if e.code in (301, 302, 307, 308) else None
        if not redirect:
            break
        import urllib.parse as _up
        url = _up.urljoin(url, redirect)
    if raw:
        return {"status": status, "body": payload.decode("utf-8", "replace")[:200]}
    try:
        parsed = json.loads(payload)
    except Exception:
        parsed = {"_raw": payload.decode("utf-8", "replace")[:200]}
    ct = ""
    for k, v in hdrs.items():
        if k.lower() == "content-type":
            ct = v
    return {"status": status, "json": normalize(parsed), "content_type": ct}


CASES = []


def case(name):
    def deco(fn):
        CASES.append((name, fn))
        return fn
    return deco


class Ctx:
    """每个后端独立的会话状态。"""

    def __init__(self, base):
        self.base = base
        self.token = None


def run_case(fn, ctx_py: Ctx, ctx_go: Ctx):
    """同一段流程分别跑两后端：fn(ctx) 返回可比对的结果列表。"""
    return fn(ctx_py), fn(ctx_go)


def diff_results(name, py_res, go_res):
    # 按步骤配对比较
    ok = True
    if len(py_res) != len(go_res):
        print(f"[{name}] ✗ 步骤数不一致: py={len(py_res)} go={len(go_res)}")
        return False
    for i, (p, g) in enumerate(zip(py_res, go_res)):
        if p != g:
            ok = False
            print(f"[{name}] ✗ step {i}:\n  py: {json.dumps(p, ensure_ascii=False)[:400]}\n  go: {json.dumps(g, ensure_ascii=False)[:400]}")
    return ok


def register_and_login(ctx: Ctx, username):
    r = call(ctx.base, "POST", "/api/v1/auth/register",
             form={"account": username, "password": "parity-sha256-pw"})
    r2 = call(ctx.base, "POST", "/api/v1/auth/login",
              form={"account": username, "password": "parity-sha256-pw"})
    if r2["status"] == 200:
        ctx.token = r2["json"]["data"]["accessToken"]
    return r, r2


def suite(ctx: Ctx, user: str):
    results = []

    # health（裸 JSON，非信封）
    results.append(call(ctx.base, "GET", "/health", raw=True))

    # auth
    r, r2 = register_and_login(ctx, user)
    results.append(r)
    results.append(r2)
    results.append(call(ctx.base, "POST", "/api/v1/auth/register",
                        form={"account": user, "password": "parity-sha256-pw"}))  # 重复注册 400
    results.append(call(ctx.base, "POST", "/api/v1/auth/login",
                        form={"account": user, "password": "wrong"}))  # 401

    # projects
    results.append(call(ctx.base, "POST", "/api/v1/projects/", body={"name": " parity项目 "}))
    results.append(call(ctx.base, "POST", "/api/v1/projects/", body={"name": " parity项目 "}))  # 重名 400
    results.append(call(ctx.base, "GET", "/api/v1/projects/"))
    results.append(call(ctx.base, "GET", "/api/v1/projects/?include_deleted=true"))
    results.append(call(ctx.base, "GET", "/api/v1/projects/not-a-uuid"))  # 422
    results.append(call(ctx.base, "GET", "/api/v1/projects/00000000-0000-0000-0000-000000000000"))  # 404

    # groups
    results.append(call(ctx.base, "POST", "/api/v1/projects/groups", body={"name": "G1"}))
    results.append(call(ctx.base, "GET", "/api/v1/projects/groups"))

    # trash（空）
    results.append(call(ctx.base, "GET", "/api/v1/projects/trash"))

    # settings
    results.append(call(ctx.base, "GET", "/api/v1/settings"))
    results.append(call(ctx.base, "GET", "/api/v1/settings/ui"))
    results.append(call(ctx.base, "GET", "/api/v1/settings/unknown"))  # 404
    results.append(call(ctx.base, "PATCH", "/api/v1/settings/editor",
                        body={"settings": {"font_size": 99}}))  # 422
    results.append(call(ctx.base, "PATCH", "/api/v1/settings/ui",
                        body={"settings": {"theme": "dark"}}))
    results.append(call(ctx.base, "DELETE", "/api/v1/settings/ui"))

    # folders
    r = call(ctx.base, "GET", "/api/v1/projects/")
    projects = r["json"]["data"]
    pid = projects[0]["id"] if projects else None
    if pid:
        results.append(call(ctx.base, "POST", "/api/v1/folders/",
                            body={"name": "F", "projectId": pid}))
        results.append(call(ctx.base, "GET", f"/api/v1/projects/{pid}/folders"))
        results.append(call(ctx.base, "GET", f"/api/v1/projects/{pid}/documents"))

    # 401 / 405 / 404 形状
    results.append(call(ctx.base, "GET", "/api/v1/settings", token=None))  # 401（未带 token）
    results.append(call(ctx.base, "POST", "/health", raw=True))  # 405 信封
    results.append(call(ctx.base, "GET", "/api/v1/ghost"))  # 404 信封
    return results


def main():
    if hasattr(sys.stdout, "reconfigure"):
        sys.stdout.reconfigure(encoding="utf-8", errors="replace")
    ap = argparse.ArgumentParser()
    ap.add_argument("--py-port", type=int, default=8000)
    ap.add_argument("--go-port", type=int, default=8001)
    args = ap.parse_args()

    if not os.path.exists(GO_BIN):
        print(f"missing {GO_BIN}; run `make build-server` first", file=sys.stderr)
        sys.exit(2)

    procs = []
    try:
        py = start_python(args.py_port)
        procs.append(py)
        go = start_go(args.go_port)
        procs.append(go)
        wait_port(args.py_port)
        wait_port(args.go_port)
        print(f"python: http://127.0.0.1:{args.py_port}   go: http://127.0.0.1:{args.go_port}")

        user = f"parity_{int(time.time() * 1000) % 100000}"
        py_res = suite(Ctx(f"http://127.0.0.1:{args.py_port}"), user)
        go_res = suite(Ctx(f"http://127.0.0.1:{args.go_port}"), user)
        steps = len(py_res)
        if diff_results("parity-suite", py_res, go_res):
            print(f"✓ 双跑一致：{steps} 个步骤全部匹配（动态字段已规范化）")
            return 0
        print("✗ 双跑存在差异，见上")
        return 1
    finally:
        for p in procs:
            if os.name == "nt":
                # uv run 会产生 uv→python 进程树；/T 连带子进程一起杀，
                # 否则残留 python 占住端口，后续运行全部 500/reset
                subprocess.run(["taskkill", "/F", "/T", "/PID", str(p.pid)], capture_output=True)
            else:
                p.send_signal(signal.SIGTERM)
                try:
                    p.wait(5)
                except Exception:
                    p.kill()


if __name__ == "__main__":
    sys.exit(main())
