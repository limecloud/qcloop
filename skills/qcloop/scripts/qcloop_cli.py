#!/usr/bin/env python3
"""qcloop 技能 CLI。

面向 AI 技能的稳定命令边界：默认输出 JSON，封装 qcloop 本地 Web/API，
让 agent 不必手写 curl 或解析自然语言输出。
"""

from __future__ import annotations

import argparse
import json
import os
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
from typing import Any, Optional

DEFAULT_BASE_URL = "http://127.0.0.1:3000"
API_FALLBACK_BASE_URL = "http://127.0.0.1:8080"
TERMINAL_STATUSES = {"completed", "failed", "paused"}
RETRYABLE_ERRORS = {"CONNECTION_FAILED", "HTTP_502", "HTTP_503", "HTTP_504"}
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
        "description": "qcloop 批次生命周期：create -> run -> status/wait -> retry。",
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
    counts = {"total": len(items), "success": 0, "failed": 0, "exhausted": 0, "awaiting_confirmation": 0, "running": 0, "pending": 0}
    for item in items:
        status = item.get("status")
        if status in counts:
            counts[status] += 1
    return counts


def problem_items(items: list[dict[str, Any]], limit: int = 20) -> list[dict[str, Any]]:
    problems: list[dict[str, Any]] = []
    for item in items:
        if item.get("status") not in {"failed", "exhausted", "awaiting_confirmation"}:
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
    base = resolve_base_url(args.base_url)
    job = request("POST", base, "/api/jobs", payload)
    run_result = None
    if args.run:
        run_result = request("POST", base, "/api/jobs/run", {"job_id": job["id"], "mode": args.mode})
    emit(envelope("job create", job, run_result=run_result, dashboard_url=f"{base}/?job_id={job['id']}"))
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


def cmd_item_answer(args: argparse.Namespace) -> int:
    payload = {
        "item_id": args.item_id,
        "answer": args.answer,
        "resume": args.resume,
    }
    result = request("POST", resolve_base_url(args.base_url), "/api/items/answer", payload)
    emit(envelope("item answer", result, item_id=args.item_id, resumed=args.resume))
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
        epilog="常用流程：doctor -> job create --file job.json --run -> job wait <job_id> -> item answer <item_id> --answer ... --resume",
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

    item = sub.add_parser("item", help="qcloop 单 item 自动化命令")
    item_sub = item.add_subparsers(dest="item_command", required=True)
    answer = item_sub.add_parser("answer", help="写回人类确认答案并可恢复 item")
    answer.add_argument("item_id")
    answer.add_argument("--answer", required=True, help="外层 AI 从人类处获得的确认答案")
    answer.add_argument("--resume", action="store_true", help="写回答案后立即重新入队")
    answer.set_defaults(func=cmd_item_answer)

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
