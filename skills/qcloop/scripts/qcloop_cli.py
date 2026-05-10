#!/usr/bin/env python3
"""qcloop 技能 CLI。

面向 AI 技能的稳定命令边界：默认输出 JSON，封装 qcloop 本地 Web/API，
让 agent 不必手写 curl 或解析自然语言输出。
"""

from __future__ import annotations

import argparse
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
from typing import Any, Optional

DEFAULT_BASE_URL = "http://127.0.0.1:3000"
API_FALLBACK_BASE_URL = "http://127.0.0.1:8080"
TERMINAL_STATUSES = {"completed", "failed", "paused", "canceled"}
RETRYABLE_ERRORS = {"CONNECTION_FAILED", "HTTP_502", "HTTP_503", "HTTP_504"}
DEFAULT_EXCLUDED_DIRS = {".git", "node_modules", "dist", "build", "target", ".next", "coverage"}
SKILL_CATALOG = [
    {
        "name": "qcloop",
        "description": "使用 qcloop Web/API 创建、启动、观察批量 QA loop。",
        "recommended_command": "qcloop-skill doctor",
        "skill_path": "skills/qcloop/SKILL.md",
        "references": ["llms-full.txt", "docs/AI_AGENT_USAGE.md"],
    },
    {
        "name": "qcloop-job",
        "description": "qcloop 批次生命周期：create -> run -> status/wait/report -> retry/cancel。",
        "recommended_command": "qcloop-skill job create --file job.json --run",
        "skill_path": "skills/qcloop/SKILL.md",
        "references": ["llms-full.txt#Core API Endpoints"],
    },
]


class CliError(Exception):
    def __init__(self, code: str, message: str, *, hint: str = "", retryable: bool = False, exit_code: int = 1):
        super().__init__(message)
        self.code = code
        self.message = message
        self.hint = hint
        self.retryable = retryable
        self.exit_code = exit_code


def resolve_base_url(value: Optional[str] = None) -> str:
    return (value or os.environ.get("QCLOOP_BASE_URL") or DEFAULT_BASE_URL).rstrip("/")


def request(method: str, base: str, path: str, payload: Any | None = None) -> Any:
    data = None
    headers = {"Accept": "application/json"}
    if payload is not None:
        data = json.dumps(payload, ensure_ascii=False).encode("utf-8")
        headers["Content-Type"] = "application/json"
    req = urllib.request.Request(f"{base}{path}", data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            body = resp.read().decode("utf-8")
    except urllib.error.HTTPError as exc:
        detail = exc.read().decode("utf-8", errors="replace").strip()
        status_code = f"HTTP_{exc.code}"
        raise CliError(
            status_code,
            f"{method} {path} failed: {detail or exc.reason}",
            hint="检查 qcloop 后端是否运行，或查看 Web 面板错误。",
            retryable=status_code in RETRYABLE_ERRORS,
            exit_code=exc.code if 1 <= exc.code <= 125 else 1,
        ) from exc
    except urllib.error.URLError as exc:
        raise CliError(
            "CONNECTION_FAILED",
            f"无法连接 qcloop: {exc.reason}",
            hint="先打开 qcloop 应用，并确认 http://127.0.0.1:3000 或 http://127.0.0.1:8080 可访问。",
            retryable=True,
            exit_code=2,
        ) from exc
    if not body:
        return None
    try:
        return json.loads(body)
    except json.JSONDecodeError as exc:
        raise CliError("INVALID_JSON", f"qcloop 返回了非 JSON 响应: {body[:300]}", exit_code=3) from exc


def read_text(base: str, path: str) -> str:
    req = urllib.request.Request(f"{base}{path}", headers={"Accept": "text/plain"})
    try:
        with urllib.request.urlopen(req, timeout=10) as resp:
            return resp.read().decode("utf-8")
    except urllib.error.URLError as exc:
        raise CliError(
            "GUIDE_UNAVAILABLE",
            f"无法读取 {path}: {exc.reason}",
            hint="确认 qcloop 前端已启动；开发模式通常是 http://127.0.0.1:3000。",
            retryable=True,
            exit_code=2,
        ) from exc


def emit(value: Any) -> None:
    print(json.dumps(value, ensure_ascii=False, indent=2))


def envelope(command: str, data: Any = None, **extra: Any) -> dict[str, Any]:
    out: dict[str, Any] = {"ok": True, "command": command}
    if data is not None:
        out["data"] = data
    out.update({key: value for key, value in extra.items() if value is not None})
    return out


def error_payload(err: CliError) -> dict[str, Any]:
    return {
        "ok": False,
        "error_code": err.code,
        "error_message": err.message,
        "retryable": err.retryable,
        "hint": err.hint,
    }


def quote(value: str) -> str:
    return urllib.parse.quote(value, safe="")


def clip(value: str, limit: int = 800) -> str:
    if len(value) <= limit:
        return value
    return value[:limit] + "\n...[truncated]"


def job_counts(items: list[dict[str, Any]]) -> dict[str, int]:
    counts = {"total": len(items), "success": 0, "failed": 0, "exhausted": 0, "awaiting_confirmation": 0, "running": 0, "pending": 0, "canceled": 0}
    for item in items:
        status = item.get("status")
        if status in counts:
            counts[status] += 1
    return counts


def problem_items(items: list[dict[str, Any]], limit: int = 20) -> list[dict[str, Any]]:
    problems: list[dict[str, Any]] = []
    for item in items:
        if item.get("status") not in {"failed", "exhausted", "awaiting_confirmation", "canceled"}:
            continue
        attempts = item.get("attempts") or []
        rounds = item.get("qc_rounds") or []
        last_attempt = attempts[-1] if attempts else {}
        last_round = rounds[-1] if rounds else {}
        problems.append(
            {
                "item_id": item.get("id"),
                "item_value": item.get("item_value"),
                "status": item.get("status"),
                "attempt_count": len(attempts),
                "qc_round_count": len(rounds),
                "last_attempt_type": last_attempt.get("attempt_type"),
                "last_exit_code": last_attempt.get("exit_code"),
                "last_stderr": clip(last_attempt.get("stderr") or ""),
                "last_feedback": clip(last_round.get("feedback") or ""),
                "confirmation_question": item.get("confirmation_question") or item.get("last_error") or "",
                "confirmation_answer": item.get("confirmation_answer") or "",
            }
        )
        if len(problems) >= limit:
            break
    return problems


def confirmation_items(items: list[dict[str, Any]], limit: int = 20) -> list[dict[str, Any]]:
    out: list[dict[str, Any]] = []
    for item in items:
        if item.get("status") != "awaiting_confirmation":
            continue
        out.append(
            {
                "item_id": item.get("id"),
                "item_value": item.get("item_value"),
                "question": item.get("confirmation_question") or item.get("last_error") or "",
                "answer": item.get("confirmation_answer") or "",
            }
        )
        if len(out) >= limit:
            break
    return out


def summarize_job(job: dict[str, Any], items: list[dict[str, Any]], *, problem_limit: int = 20) -> dict[str, Any]:
    return {
        "job_id": job.get("id"),
        "name": job.get("name"),
        "status": job.get("status"),
        "run_no": job.get("run_no"),
        "max_qc_rounds": job.get("max_qc_rounds"),
        "max_executor_retries": job.get("max_executor_retries"),
        "executor_provider": job.get("executor_provider"),
        "execution_mode": job.get("execution_mode"),
        "created_at": job.get("created_at"),
        "finished_at": job.get("finished_at"),
        "counts": job_counts(items),
        "problems": problem_items(items, problem_limit),
        "awaiting_confirmations": confirmation_items(items, problem_limit),
    }


def load_job_and_items(base: str, job_id: str) -> tuple[dict[str, Any], list[dict[str, Any]]]:
    job = request("GET", base, f"/api/jobs/{quote(job_id)}")
    items = request("GET", base, f"/api/items/?job_id={quote(job_id)}") or []
    return job, items


def parse_items_text(text: str | None) -> list[str]:
    return [line.strip() for line in (text or "").splitlines() if line.strip()]


def should_skip_dir(path: Path) -> bool:
    return path.name in DEFAULT_EXCLUDED_DIRS


def walk_files(root: Path) -> list[Path]:
    if not root.exists():
        return []
    out: list[Path] = []
    for current, dirs, files in os.walk(root):
        dirs[:] = [name for name in dirs if name not in DEFAULT_EXCLUDED_DIRS]
        current_path = Path(current)
        if should_skip_dir(current_path):
            continue
        for name in files:
            out.append(current_path / name)
    return sorted(out)


def collect_glob_files(cwd: Path, patterns: list[str]) -> list[Path]:
    if not patterns:
        return []
    regexes = [glob_to_regex(pattern) for pattern in patterns]
    out: list[Path] = []
    for file in walk_files(cwd):
        rel = file.relative_to(cwd).as_posix()
        if any(regex.match(rel) for regex in regexes):
            out.append(file)
    return out


def glob_to_regex(pattern: str) -> re.Pattern[str]:
    normalized = pattern.replace(os.sep, "/")
    out = "^"
    i = 0
    while i < len(normalized):
        ch = normalized[i]
        if ch == "*":
            if i + 1 < len(normalized) and normalized[i + 1] == "*":
                slash_after = i + 2 < len(normalized) and normalized[i + 2] == "/"
                out += "(?:.*/)?" if slash_after else ".*"
                i += 3 if slash_after else 2
                continue
            out += "[^/]*"
        elif ch == "?":
            out += "[^/]"
        else:
            out += re.escape(ch)
        i += 1
    out += "$"
    return re.compile(out)


def collect_git_diff_files(cwd: Path, ref: str | None) -> list[Path]:
    cmd = ["git", "-C", str(cwd), "diff", "--name-only"]
    if ref:
        cmd.append(ref)
    output = subprocess.check_output(cmd, text=True)
    files: list[Path] = []
    for line in output.splitlines():
        rel = line.strip()
        if not rel:
            continue
        file = cwd / rel
        if file.exists():
            files.append(file)
    return files


def structured_item(file: Path, cwd: Path, source: str) -> str:
    target = file.relative_to(cwd).as_posix() if file.is_relative_to(cwd) else str(file)
    return json.dumps(
        {
            "name": target,
            "target": target,
            "cwd": str(cwd),
            "source": source,
            "expected": "由外层 AI 根据当前任务意图执行该 item，并输出修改文件、验证命令、结果和风险。",
        },
        ensure_ascii=False,
        separators=(",", ":"),
    )


def build_imported_items(args: argparse.Namespace) -> list[str]:
    cwd = Path(args.cwd or os.getcwd()).resolve()
    files: dict[Path, str] = {}
    for item_dir in args.items_dir or []:
        root = (cwd / item_dir).resolve()
        for file in walk_files(root):
            files[file] = "items-dir"
    for file in collect_glob_files(cwd, args.glob or []):
        files[file] = "glob"
    if args.git_diff is not None:
        for file in collect_git_diff_files(cwd, args.git_diff):
            files[file] = "git-diff"
    return [structured_item(file, cwd, source) for file, source in files.items()]


def merge_imported_items(payload: dict[str, Any], imported_items: list[str]) -> dict[str, Any]:
    if not imported_items:
        return payload
    existing = payload.get("items") if isinstance(payload.get("items"), list) else parse_items_text(payload.get("items_text"))
    out = dict(payload)
    out["items"] = [*existing, *imported_items]
    out.pop("items_text", None)
    return out


def duration_from_dates(start: str | None, end: str | None) -> str:
    if not start:
        return "-"
    try:
        started = time.mktime(time.strptime(start[:19], "%Y-%m-%dT%H:%M:%S"))
        finished = time.mktime(time.strptime((end or time.strftime("%Y-%m-%dT%H:%M:%S"))[:19], "%Y-%m-%dT%H:%M:%S"))
    except ValueError:
        return "-"
    seconds = max(0, int(finished - started))
    hours, rest = divmod(seconds, 3600)
    minutes, sec = divmod(rest, 60)
    if hours:
        return f"{hours}h {minutes}m"
    if minutes:
        return f"{minutes}m {sec}s"
    return f"{sec}s"


def markdown_report(data: dict[str, Any]) -> str:
    counts = data.get("counts") or {}
    lines = [
        f"# qcloop 托管报告: {data.get('name') or data.get('job_id')}",
        "",
        f"- 批次 ID: {data.get('job_id')}",
        f"- 状态: {data.get('status')}",
        f"- 运行轮次: {data.get('run_no')}",
        f"- 最大质检轮次: {data.get('max_qc_rounds')}",
        f"- 执行器失败自动重试: {data.get('max_executor_retries') or 0}",
        f"- 已耗时: {duration_from_dates(data.get('created_at'), data.get('finished_at'))}",
        "",
        "| 指标 | 数量 |",
        "| --- | ---: |",
    ]
    for key in ["total", "success", "failed", "exhausted", "awaiting_confirmation", "running", "pending", "canceled"]:
        lines.append(f"| {key} | {counts.get(key, 0)} |")
    if data.get("awaiting_confirmations"):
        lines.extend(["", "## 待确认"])
        for item in data["awaiting_confirmations"]:
            lines.append(f"- {item.get('item_id')}: {item.get('question') or item.get('item_value')}")
    if data.get("problems"):
        lines.extend(["", "## 问题项"])
        for item in data["problems"]:
            lines.append(f"- {item.get('item_id')} [{item.get('status')}] {item.get('item_value')}")
            if item.get("last_feedback"):
                lines.append(f"  - 质检反馈: {str(item.get('last_feedback')).replace(chr(10), ' ')}")
            if item.get("last_stderr"):
                lines.append(f"  - stderr: {str(item.get('last_stderr')).replace(chr(10), ' ')}")
            if item.get("confirmation_question"):
                lines.append(f"  - 待确认: {str(item.get('confirmation_question')).replace(chr(10), ' ')}")
    return "\n".join(lines) + "\n"


def cmd_doctor(args: argparse.Namespace) -> int:
    candidates = [resolve_base_url(args.base_url)]
    if candidates[0] != API_FALLBACK_BASE_URL:
        candidates.append(API_FALLBACK_BASE_URL)

    checks = []
    selected = None
    for base in candidates:
        result: dict[str, Any] = {"base_url": base}
        try:
            guide = read_text(base, "/llm-full.txt")
            result["llm_full"] = {"ok": True, "bytes": len(guide)}
        except CliError as exc:
            result["llm_full"] = {"ok": False, "error_code": exc.code, "error_message": exc.message}
        try:
            jobs = request("GET", base, "/api/jobs")
            result["api_jobs"] = {"ok": True, "count": len(jobs or [])}
        except CliError as exc:
            result["api_jobs"] = {"ok": False, "error_code": exc.code, "error_message": exc.message}
        checks.append(result)
        if result["api_jobs"].get("ok"):
            selected = base
            break

    emit(envelope("doctor", {"selected_base_url": selected, "checks": checks}, ready=selected is not None))
    return 0 if selected else 2


def cmd_guide(args: argparse.Namespace) -> int:
    path = "/llm-full.txt" if args.full else "/llm.txt"
    text = read_text(resolve_base_url(args.base_url), path)
    if args.raw:
        print(text)
    else:
        emit(envelope("guide", {"path": path, "bytes": len(text), "text": text}))
    return 0


def cmd_job_list(args: argparse.Namespace) -> int:
    jobs = request("GET", resolve_base_url(args.base_url), "/api/jobs") or []
    if args.status:
        jobs = [job for job in jobs if job.get("status") == args.status]
    if args.limit is not None:
        jobs = jobs[: args.limit]
    emit(envelope("job list", jobs, count=len(jobs)))
    return 0


def cmd_job_create(args: argparse.Namespace) -> int:
    with open(args.file, "r", encoding="utf-8") as fh:
        payload = json.load(fh)
    imported_items = build_imported_items(args)
    payload = merge_imported_items(payload, imported_items)
    base = resolve_base_url(args.base_url)
    job = request("POST", base, "/api/jobs", payload)
    run_result = None
    if args.run:
        run_result = request("POST", base, "/api/jobs/run", {"job_id": job["id"], "mode": args.mode})
    emit(envelope("job create", job, run_result=run_result, imported_item_count=len(imported_items), dashboard_url=f"{base}/?job_id={job['id']}"))
    return 0


def cmd_job_run(args: argparse.Namespace) -> int:
    result = request("POST", resolve_base_url(args.base_url), "/api/jobs/run", {"job_id": args.job_id, "mode": args.mode})
    emit(envelope("job run", result, job_id=args.job_id, mode=args.mode))
    return 0


def cmd_job_pause(args: argparse.Namespace) -> int:
    result = request("POST", resolve_base_url(args.base_url), "/api/jobs/pause", {"job_id": args.job_id})
    emit(envelope("job pause", result, job_id=args.job_id))
    return 0


def cmd_job_resume(args: argparse.Namespace) -> int:
    result = request("POST", resolve_base_url(args.base_url), "/api/jobs/resume", {"job_id": args.job_id})
    emit(envelope("job resume", result, job_id=args.job_id))
    return 0


def cmd_job_cancel(args: argparse.Namespace) -> int:
    result = request("POST", resolve_base_url(args.base_url), "/api/jobs/cancel", {"job_id": args.job_id, "reason": args.reason})
    emit(envelope("job cancel", result, job_id=args.job_id))
    return 0


def cmd_job_status(args: argparse.Namespace) -> int:
    job, items = load_job_and_items(resolve_base_url(args.base_url), args.job_id)
    data: dict[str, Any] = summarize_job(job, items, problem_limit=args.problem_limit)
    if args.include_items:
        data["items"] = items
    emit(envelope("job status", data))
    return 0


def cmd_job_wait(args: argparse.Namespace) -> int:
    base = resolve_base_url(args.base_url)
    deadline = time.monotonic() + args.timeout
    data: dict[str, Any] | None = None
    while True:
        job, items = load_job_and_items(base, args.job_id)
        data = summarize_job(job, items, problem_limit=args.problem_limit)
        if data["counts"].get("awaiting_confirmation", 0) > 0:
            emit(envelope("job wait", data, terminal=False, needs_confirmation=True, dashboard_url=f"{base}/?job_id={args.job_id}"))
            return 3
        if job.get("status") in TERMINAL_STATUSES:
            emit(envelope("job wait", data, terminal=True, dashboard_url=f"{base}/?job_id={args.job_id}"))
            return 0 if job.get("status") == "completed" else 2
        if time.monotonic() >= deadline:
            emit(envelope("job wait", data, terminal=False, timed_out=True, dashboard_url=f"{base}/?job_id={args.job_id}"))
            return 124
        time.sleep(args.interval)


def cmd_job_report(args: argparse.Namespace) -> int:
    base = resolve_base_url(args.base_url)
    job, items = load_job_and_items(base, args.job_id)
    data: dict[str, Any] = summarize_job(job, items, problem_limit=args.problem_limit)
    if args.format == "markdown":
        data["markdown"] = markdown_report(data)
    elif args.format != "json":
        raise CliError("INVALID_ARGUMENT", "--format 只能是 json 或 markdown", exit_code=64)
    emit(envelope("job report", data, dashboard_url=f"{base}/?job_id={args.job_id}"))
    return 0


def cmd_item_answer(args: argparse.Namespace) -> int:
    payload = {
        "item_id": args.item_id,
        "answer": args.answer,
        "resume": args.resume,
    }
    result = request("POST", resolve_base_url(args.base_url), "/api/items/answer", payload)
    emit(envelope("item answer", result, item_id=args.item_id, resumed=args.resume))
    return 0


def cmd_item_retry(args: argparse.Namespace) -> int:
    result = request("POST", resolve_base_url(args.base_url), "/api/items/retry", {"item_id": args.item_id})
    emit(envelope("item retry", result, item_id=args.item_id))
    return 0


def cmd_item_cancel(args: argparse.Namespace) -> int:
    result = request("POST", resolve_base_url(args.base_url), "/api/items/cancel", {"item_id": args.item_id, "reason": args.reason})
    emit(envelope("item cancel", result, item_id=args.item_id))
    return 0


def cmd_template_list(args: argparse.Namespace) -> int:
    templates = request("GET", resolve_base_url(args.base_url), "/api/templates") or []
    emit(envelope("template list", templates, count=len(templates)))
    return 0


def cmd_template_show(args: argparse.Namespace) -> int:
    template = request("GET", resolve_base_url(args.base_url), f"/api/templates/{quote(args.template_id)}")
    emit(envelope("template show", template))
    return 0


def cmd_template_create(args: argparse.Namespace) -> int:
    with open(args.file, "r", encoding="utf-8") as fh:
        payload = json.load(fh)
    template = request("POST", resolve_base_url(args.base_url), "/api/templates", payload)
    emit(envelope("template create", template))
    return 0


def cmd_template_update(args: argparse.Namespace) -> int:
    with open(args.file, "r", encoding="utf-8") as fh:
        payload = json.load(fh)
    template = request("PUT", resolve_base_url(args.base_url), f"/api/templates/{quote(args.template_id)}", payload)
    emit(envelope("template update", template))
    return 0


def cmd_template_delete(args: argparse.Namespace) -> int:
    result = request("DELETE", resolve_base_url(args.base_url), f"/api/templates/{quote(args.template_id)}")
    emit(envelope("template delete", result, template_id=args.template_id))
    return 0


def cmd_queue_metrics(args: argparse.Namespace) -> int:
    metrics = request("GET", resolve_base_url(args.base_url), "/api/queue/metrics")
    emit(envelope("queue metrics", metrics))
    return 0


def cmd_skill_list(args: argparse.Namespace) -> int:
    emit(envelope("skill list", SKILL_CATALOG, count=len(SKILL_CATALOG)))
    return 0


def cmd_skill_show(args: argparse.Namespace) -> int:
    normalized = args.name.strip().lower()
    for item in SKILL_CATALOG:
        if item["name"].lower() == normalized:
            emit(envelope("skill show", item))
            return 0
    raise CliError("SKILL_NOT_FOUND", f"未知 skill: {args.name}", hint="运行 qcloop-skill skill list 查看可用项。", exit_code=4)


def cmd_api(args: argparse.Namespace) -> int:
    body = None
    if args.file:
        with open(args.file, "r", encoding="utf-8") as fh:
            body = json.load(fh)
    elif args.data:
        body = json.loads(args.data)
    data = request(args.method.upper(), resolve_base_url(args.base_url), args.path, body)
    emit(envelope("api", data, method=args.method.upper(), path=args.path))
    return 0


def add_common(parser: argparse.ArgumentParser) -> None:
    parser.add_argument("--base-url", help=f"qcloop Web/API 基础地址，默认 {DEFAULT_BASE_URL}；也可用 QCLOOP_BASE_URL")


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="qcloop-skill",
        description="qcloop 技能 CLI，默认输出结构化 JSON，供 AI 智能体/技能调用。",
        epilog="常用流程：doctor -> job create --file job.json --run -> job wait <job_id> -> job report <job_id> --format markdown -> item answer <item_id> --answer ... --resume",
    )
    add_common(parser)
    sub = parser.add_subparsers(dest="command", required=True)

    doctor = sub.add_parser("doctor", help="检查 qcloop guide 和 API 是否可访问")
    doctor.set_defaults(func=cmd_doctor)

    guide = sub.add_parser("guide", help="读取 llm.txt 或 llm-full.txt")
    guide.add_argument("--full", action="store_true", help="读取 /llm-full.txt")
    guide.add_argument("--raw", action="store_true", help="直接输出文本，不包 JSON")
    guide.set_defaults(func=cmd_guide)

    job = sub.add_parser("job", help="qcloop 批次生命周期命令")
    job_sub = job.add_subparsers(dest="job_command", required=True)

    job_list = job_sub.add_parser("list", help="列出批次")
    job_list.add_argument("--status", help="按状态过滤")
    job_list.add_argument("--limit", type=int, help="最多返回多少条")
    job_list.set_defaults(func=cmd_job_list)

    create = job_sub.add_parser("create", help="从 JSON 文件创建批次")
    create.add_argument("--file", required=True, help="批次 payload JSON 文件")
    create.add_argument("--run", action="store_true", help="创建后立即入队")
    create.add_argument("--mode", default="auto", help="启动模式：auto/continue/retry_unfinished/rerun_all")
    create.add_argument("--cwd", help="导入 items 时使用的项目根目录，默认当前目录")
    create.add_argument("--items-dir", action="append", help="把目录下文件导入为结构化 items，可重复")
    create.add_argument("--glob", action="append", help="按 glob 导入文件，如 **/*.md，可重复")
    create.add_argument("--git-diff", help="导入 git diff --name-only <ref> 的文件")
    create.set_defaults(func=cmd_job_create)

    run = job_sub.add_parser("run", help="启动、继续或重跑批次")
    run.add_argument("job_id")
    run.add_argument("--mode", default="auto", help="auto/continue/retry_unfinished/rerun_all")
    run.set_defaults(func=cmd_job_run)

    pause = job_sub.add_parser("pause", help="暂停批次")
    pause.add_argument("job_id")
    pause.set_defaults(func=cmd_job_pause)

    resume = job_sub.add_parser("resume", help="恢复暂停批次")
    resume.add_argument("job_id")
    resume.set_defaults(func=cmd_job_resume)

    cancel = job_sub.add_parser("cancel", help="取消批次，进入不可恢复终态")
    cancel.add_argument("job_id")
    cancel.add_argument("--reason", default="外层 AI 取消批处理")
    cancel.set_defaults(func=cmd_job_cancel)

    status = job_sub.add_parser("status", help="读取批次摘要和失败证据")
    status.add_argument("job_id")
    status.add_argument("--include-items", action="store_true", help="包含完整 items/attempts/qc_rounds")
    status.add_argument("--problem-limit", type=int, default=20, help="最多展开多少个失败/耗尽项")
    status.set_defaults(func=cmd_job_status)

    wait = job_sub.add_parser("wait", help="等待批次进入终态")
    wait.add_argument("job_id")
    wait.add_argument("--timeout", type=int, default=1800)
    wait.add_argument("--interval", type=float, default=5.0)
    wait.add_argument("--problem-limit", type=int, default=20)
    wait.set_defaults(func=cmd_job_wait)

    report = job_sub.add_parser("report", help="生成睡前托管报告")
    report.add_argument("job_id")
    report.add_argument("--format", choices=["json", "markdown"], default="json")
    report.add_argument("--problem-limit", type=int, default=50)
    report.set_defaults(func=cmd_job_report)

    item = sub.add_parser("item", help="qcloop 单 item 自动化命令")
    item_sub = item.add_subparsers(dest="item_command", required=True)
    answer = item_sub.add_parser("answer", help="写回人类确认答案并可恢复 item")
    answer.add_argument("item_id")
    answer.add_argument("--answer", required=True, help="外层 AI 从人类处获得的确认答案")
    answer.add_argument("--resume", action="store_true", help="写回答案后立即重新入队")
    answer.set_defaults(func=cmd_item_answer)
    retry = item_sub.add_parser("retry", help="重试单个 item")
    retry.add_argument("item_id")
    retry.set_defaults(func=cmd_item_retry)
    cancel_item = item_sub.add_parser("cancel", help="取消单个 item")
    cancel_item.add_argument("item_id")
    cancel_item.add_argument("--reason", default="外层 AI 取消单个 item")
    cancel_item.set_defaults(func=cmd_item_cancel)

    template = sub.add_parser("template", help="管理 qcloop 批次模板")
    template_sub = template.add_subparsers(dest="template_command", required=True)
    template_list = template_sub.add_parser("list", help="列出批次模板")
    template_list.set_defaults(func=cmd_template_list)
    template_show = template_sub.add_parser("show", help="显示批次模板")
    template_show.add_argument("template_id")
    template_show.set_defaults(func=cmd_template_show)
    template_create = template_sub.add_parser("create", help="从 JSON 文件创建批次模板")
    template_create.add_argument("--file", required=True, help="模板 payload JSON 文件")
    template_create.set_defaults(func=cmd_template_create)
    template_update = template_sub.add_parser("update", help="更新批次模板")
    template_update.add_argument("template_id")
    template_update.add_argument("--file", required=True, help="模板 payload JSON 文件")
    template_update.set_defaults(func=cmd_template_update)
    template_delete = template_sub.add_parser("delete", help="删除批次模板")
    template_delete.add_argument("template_id")
    template_delete.set_defaults(func=cmd_template_delete)

    queue = sub.add_parser("queue", help="查看 qcloop 队列指标")
    queue_sub = queue.add_subparsers(dest="queue_command", required=True)
    metrics = queue_sub.add_parser("metrics", help="输出队列指标")
    metrics.set_defaults(func=cmd_queue_metrics)

    skill = sub.add_parser("skill", help="查看 qcloop 技能目录")
    skill_sub = skill.add_subparsers(dest="skill_command", required=True)
    skill_list = skill_sub.add_parser("list", help="列出技能能力")
    skill_list.set_defaults(func=cmd_skill_list)
    skill_show = skill_sub.add_parser("show", help="显示单个技能能力")
    skill_show.add_argument("name")
    skill_show.set_defaults(func=cmd_skill_show)

    api = sub.add_parser("api", help="原始 API 逃生口，默认只建议 GET/只读排障")
    api.add_argument("method", help="HTTP method，如 GET/POST")
    api.add_argument("path", help="API path，如 /api/jobs")
    api.add_argument("--data", help="JSON 字符串请求体")
    api.add_argument("--file", help="JSON 文件请求体")
    api.set_defaults(func=cmd_api)

    return parser


def main() -> int:
    parser = build_parser()
    args = parser.parse_args()
    try:
        return args.func(args)
    except CliError as err:
        print(json.dumps(error_payload(err), ensure_ascii=False, indent=2), file=sys.stderr)
        return err.exit_code
    except KeyboardInterrupt:
        print(json.dumps(error_payload(CliError("INTERRUPTED", "用户中断", exit_code=130)), ensure_ascii=False, indent=2), file=sys.stderr)
        return 130


if __name__ == "__main__":
    sys.exit(main())
