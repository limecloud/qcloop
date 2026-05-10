#!/usr/bin/env node

const fs = require("fs");
const path = require("path");
const childProcess = require("child_process");

const DEFAULT_BASE_URL = "http://127.0.0.1:3000";
const API_FALLBACK_BASE_URL = "http://127.0.0.1:8080";
const TERMINAL_STATUSES = new Set(["completed", "failed", "paused", "canceled"]);
const RETRYABLE_ERRORS = new Set(["CONNECTION_FAILED", "HTTP_502", "HTTP_503", "HTTP_504"]);
const DEFAULT_EXCLUDED_DIRS = new Set([".git", "node_modules", "dist", "build", "target", ".next", "coverage"]);
const SKILL_CATALOG = [
  {
    name: "qcloop",
    description: "使用 qcloop Web/API 创建、启动、观察批量 QA loop。",
    recommended_command: "qcloop-skill doctor",
    skill_path: "skills/qcloop/SKILL.md",
    references: ["llms-full.txt", "docs/AI_AGENT_USAGE.md"],
  },
  {
    name: "qcloop-job",
    description: "qcloop 批次生命周期：create -> run -> status/wait/report -> retry/cancel。",
    recommended_command: "qcloop-skill job create --file job.json --run",
    skill_path: "skills/qcloop/SKILL.md",
    references: ["llms-full.txt#Core API Endpoints"],
  },
];

class CliError extends Error {
  constructor(code, message, options = {}) {
    super(message);
    this.code = code;
    this.hint = options.hint || "";
    this.retryable = Boolean(options.retryable);
    this.exitCode = options.exitCode || 1;
  }
}

function rootHelp() {
  return `qcloop 技能 CLI，默认输出结构化 JSON，供 AI 智能体/技能调用。

Usage:
  qcloop-skill [--base-url <url>] <command> [args]

Commands:
  doctor                 检查 qcloop guide 和 API 是否可访问
  guide                  读取 llm.txt 或 llm-full.txt
  job                    qcloop 批次生命周期命令
  item                   写回待确认 item 的人类确认答案
  template               管理批次模板
  queue                  查看队列指标
  skill                  查看 qcloop 技能目录
  api                    原始 API 逃生口，默认只建议 GET/只读排障

Options:
  --base-url <url>       qcloop Web/API 基础地址，默认 ${DEFAULT_BASE_URL}
  --json                 兼容参数；本 CLI 默认输出 JSON
  -h, --help             显示帮助

常用流程：
  qcloop-skill doctor
  qcloop-skill job create --file job.json --run
  qcloop-skill job wait <job_id>
  qcloop-skill job report <job_id> --format markdown`;
}

function jobHelp() {
  return `Usage:
  qcloop-skill job list [--status <status>] [--limit <n>]
  qcloop-skill job create --file <payload.json> [--run] [--mode auto] [--items-dir <dir>] [--glob "**/*.md"] [--git-diff <ref>]
  qcloop-skill job run <job_id> [--mode retry_unfinished]
  qcloop-skill job pause <job_id>
  qcloop-skill job resume <job_id>
  qcloop-skill job cancel <job_id> [--reason <text>]
  qcloop-skill job status <job_id> [--include-items] [--problem-limit <n>]
  qcloop-skill job wait <job_id> [--timeout 1800] [--interval 5]
  qcloop-skill job report <job_id> [--format json|markdown] [--problem-limit <n>]`;
}

function itemHelp() {
  return `Usage:
  qcloop-skill item answer <item_id> --answer <text> [--resume]
  qcloop-skill item retry <item_id>
  qcloop-skill item cancel <item_id> [--reason <text>]`;
}

function templateHelp() {
  return `Usage:
  qcloop-skill template list
  qcloop-skill template show <template_id>
  qcloop-skill template create --file <template.json>
  qcloop-skill template update <template_id> --file <template.json>
  qcloop-skill template delete <template_id>`;
}

function queueHelp() {
  return `Usage:
  qcloop-skill queue metrics`;
}

function skillHelp() {
  return `Usage:
  qcloop-skill skill list
  qcloop-skill skill show <name>`;
}

function resolveBaseUrl(value) {
  return (value || process.env.QCLOOP_BASE_URL || DEFAULT_BASE_URL).replace(/\/+$/, "");
}

function emit(value) {
  process.stdout.write(`${JSON.stringify(value, null, 2)}\n`);
}

function emitError(error) {
  const payload = {
    ok: false,
    error_code: error.code || "ERROR",
    error_message: error.message,
    retryable: Boolean(error.retryable),
    hint: error.hint || "",
  };
  process.stderr.write(`${JSON.stringify(payload, null, 2)}\n`);
}

function envelope(command, data, extra = {}) {
  const payload = { ok: true, command };
  if (data !== undefined) payload.data = data;
  for (const [key, value] of Object.entries(extra)) {
    if (value !== undefined) payload[key] = value;
  }
  return payload;
}

function takeFlag(args, name) {
  const index = args.indexOf(name);
  if (index === -1) return false;
  args.splice(index, 1);
  return true;
}

function takeOption(args, name, fallback) {
  const index = args.indexOf(name);
  if (index === -1) return fallback;
  const value = args[index + 1];
  if (!value || value.startsWith("--")) {
    throw new CliError("INVALID_ARGUMENT", `缺少 ${name} 的参数`, { exitCode: 64 });
  }
  args.splice(index, 2);
  return value;
}

function takeOptions(args, name) {
  const values = [];
  while (true) {
    const index = args.indexOf(name);
    if (index === -1) return values;
    const value = args[index + 1];
    if (!value || value.startsWith("--")) {
      throw new CliError("INVALID_ARGUMENT", `缺少 ${name} 的参数`, { exitCode: 64 });
    }
    values.push(value);
    args.splice(index, 2);
  }
}

function parseIntOption(args, name, fallback) {
  const raw = takeOption(args, name, undefined);
  if (raw === undefined) return fallback;
  const value = Number.parseInt(raw, 10);
  if (!Number.isFinite(value) || value < 0) {
    throw new CliError("INVALID_ARGUMENT", `${name} 必须是非负整数`, { exitCode: 64 });
  }
  return value;
}

function parseFloatOption(args, name, fallback) {
  const raw = takeOption(args, name, undefined);
  if (raw === undefined) return fallback;
  const value = Number.parseFloat(raw);
  if (!Number.isFinite(value) || value <= 0) {
    throw new CliError("INVALID_ARGUMENT", `${name} 必须是正数`, { exitCode: 64 });
  }
  return value;
}

function parseGlobalArgs(argv) {
  const args = [...argv];
  const help = takeFlag(args, "--help") || takeFlag(args, "-h");
  takeFlag(args, "--json");
  const baseUrl = takeOption(args, "--base-url", undefined);
  return { args, help, baseUrl: resolveBaseUrl(baseUrl) };
}

function quote(value) {
  return encodeURIComponent(value);
}

function clip(value, limit = 800) {
  const text = value || "";
  return text.length <= limit ? text : `${text.slice(0, limit)}\n...[truncated]`;
}

async function fetchWithTimeout(url, options = {}, timeoutMs = 30000) {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), timeoutMs);
  try {
    return await fetch(url, { ...options, signal: controller.signal });
  } catch (error) {
    if (error.name === "AbortError") {
      throw new CliError("TIMEOUT", `请求超时: ${url}`, {
        hint: "检查 qcloop 服务是否卡住，或稍后重试。",
        retryable: true,
        exitCode: 124,
      });
    }
    throw new CliError("CONNECTION_FAILED", `无法连接 qcloop: ${error.message}`, {
      hint: `先打开 qcloop 应用，并确认 ${DEFAULT_BASE_URL} 或 ${API_FALLBACK_BASE_URL} 可访问。`,
      retryable: true,
      exitCode: 2,
    });
  } finally {
    clearTimeout(timer);
  }
}

async function request(method, baseUrl, path, payload) {
  const headers = { Accept: "application/json" };
  const options = { method, headers };
  if (payload !== undefined && payload !== null) {
    headers["Content-Type"] = "application/json";
    options.body = JSON.stringify(payload);
  }
  const response = await fetchWithTimeout(`${baseUrl}${path}`, options);
  const body = await response.text();
  if (!response.ok) {
    const code = `HTTP_${response.status}`;
    throw new CliError(code, `${method} ${path} failed: ${body || response.statusText}`, {
      hint: "检查 qcloop 后端是否运行，或查看 Web 面板错误。",
      retryable: RETRYABLE_ERRORS.has(code),
      exitCode: response.status >= 1 && response.status <= 125 ? response.status : 1,
    });
  }
  if (!body) return null;
  try {
    return JSON.parse(body);
  } catch (error) {
    throw new CliError("INVALID_JSON", `qcloop 返回了非 JSON 响应: ${body.slice(0, 300)}`, { exitCode: 3 });
  }
}

async function readText(baseUrl, path) {
  const response = await fetchWithTimeout(`${baseUrl}${path}`, { method: "GET", headers: { Accept: "text/plain" } }, 10000);
  const body = await response.text();
  if (!response.ok) {
    throw new CliError(`HTTP_${response.status}`, `无法读取 ${path}: ${body || response.statusText}`, {
      hint: `确认 qcloop 前端已启动；开发模式通常是 ${DEFAULT_BASE_URL}。`,
      retryable: RETRYABLE_ERRORS.has(`HTTP_${response.status}`),
      exitCode: response.status >= 1 && response.status <= 125 ? response.status : 1,
    });
  }
  return body;
}

function jobCounts(items) {
  const counts = { total: items.length, success: 0, failed: 0, exhausted: 0, awaiting_confirmation: 0, running: 0, pending: 0, canceled: 0 };
  for (const item of items) {
    if (Object.prototype.hasOwnProperty.call(counts, item.status)) counts[item.status] += 1;
  }
  return counts;
}

function problemItems(items, limit = 20) {
  const problems = [];
  for (const item of items) {
    if (!["failed", "exhausted", "awaiting_confirmation", "canceled"].includes(item.status)) continue;
    const attempts = item.attempts || [];
    const rounds = item.qc_rounds || [];
    const lastAttempt = attempts.at(-1) || {};
    const lastRound = rounds.at(-1) || {};
    problems.push({
      item_id: item.id,
      item_value: item.item_value,
      status: item.status,
      attempt_count: attempts.length,
      qc_round_count: rounds.length,
      last_attempt_type: lastAttempt.attempt_type,
      last_exit_code: lastAttempt.exit_code,
      last_stderr: clip(lastAttempt.stderr || ""),
      last_feedback: clip(lastRound.feedback || ""),
      confirmation_question: item.confirmation_question || item.last_error || "",
      confirmation_answer: item.confirmation_answer || "",
    });
    if (problems.length >= limit) break;
  }
  return problems;
}

function confirmationItems(items, limit = 20) {
  const out = [];
  for (const item of items) {
    if (item.status !== "awaiting_confirmation") continue;
    out.push({
      item_id: item.id,
      item_value: item.item_value,
      question: item.confirmation_question || item.last_error || "",
      answer: item.confirmation_answer || "",
    });
    if (out.length >= limit) break;
  }
  return out;
}

function summarizeJob(job, items, problemLimit = 20) {
  return {
    job_id: job.id,
    name: job.name,
    status: job.status,
    run_no: job.run_no,
    max_qc_rounds: job.max_qc_rounds,
    max_executor_retries: job.max_executor_retries,
    executor_provider: job.executor_provider,
    execution_mode: job.execution_mode,
    created_at: job.created_at,
    finished_at: job.finished_at,
    counts: jobCounts(items),
    problems: problemItems(items, problemLimit),
    awaiting_confirmations: confirmationItems(items, problemLimit),
  };
}

function parseItemsText(text) {
  return String(text || "")
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter(Boolean);
}

function shouldSkipDir(name) {
  return DEFAULT_EXCLUDED_DIRS.has(name);
}

function walkFiles(root) {
  const out = [];
  const stack = [root];
  while (stack.length > 0) {
    const current = stack.pop();
    let entries;
    try {
      entries = fs.readdirSync(current, { withFileTypes: true });
    } catch {
      continue;
    }
    for (const entry of entries) {
      const full = path.join(current, entry.name);
      if (entry.isDirectory()) {
        if (!shouldSkipDir(entry.name)) stack.push(full);
      } else if (entry.isFile()) {
        out.push(full);
      }
    }
  }
  return out.sort();
}

function globToRegExp(pattern) {
  const normalized = pattern.split(path.sep).join("/");
  let out = "^";
  for (let i = 0; i < normalized.length; i++) {
    const ch = normalized[i];
    if (ch === "*") {
      if (normalized[i + 1] === "*") {
        const slashAfter = normalized[i + 2] === "/";
        out += slashAfter ? "(?:.*/)?" : ".*";
        i += slashAfter ? 2 : 1;
      } else {
        out += "[^/]*";
      }
    } else if (ch === "?") {
      out += "[^/]";
    } else {
      out += ch.replace(/[|\\{}()[\]^$+?.]/g, "\\$&");
    }
  }
  out += "$";
  return new RegExp(out);
}

function collectGlobFiles(cwd, patterns) {
  if (patterns.length === 0) return [];
  const regexes = patterns.map(globToRegExp);
  return walkFiles(cwd).filter((file) => {
    const rel = path.relative(cwd, file).split(path.sep).join("/");
    return regexes.some((regex) => regex.test(rel));
  });
}

function collectGitDiffFiles(cwd, ref) {
  const args = ["-C", cwd, "diff", "--name-only"];
  if (ref) args.push(ref);
  const output = childProcess.execFileSync("git", args, { encoding: "utf8" });
  return output
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter(Boolean)
    .map((file) => path.resolve(cwd, file))
    .filter((file) => fs.existsSync(file));
}

function structuredItem(file, cwd, source) {
  const target = path.relative(cwd, file).split(path.sep).join("/");
  return JSON.stringify({
    name: target,
    target,
    cwd,
    source,
    expected: "由外层 AI 根据当前任务意图执行该 item，并输出修改文件、验证命令、结果和风险。",
  });
}

function buildImportedItems(args) {
  const cwd = path.resolve(takeOption(args, "--cwd", process.cwd()));
  const itemDirs = takeOptions(args, "--items-dir");
  const globs = takeOptions(args, "--glob");
  const gitDiffRef = takeOption(args, "--git-diff", undefined);
  const files = new Map();

  for (const dir of itemDirs) {
    const root = path.resolve(cwd, dir);
    for (const file of walkFiles(root)) files.set(file, "items-dir");
  }
  for (const file of collectGlobFiles(cwd, globs)) files.set(file, "glob");
  if (gitDiffRef !== undefined) {
    for (const file of collectGitDiffFiles(cwd, gitDiffRef)) files.set(file, "git-diff");
  }

  return Array.from(files.entries()).map(([file, source]) => structuredItem(file, cwd, source));
}

function mergeImportedItems(payload, importedItems) {
  if (importedItems.length === 0) return payload;
  const existingItems = Array.isArray(payload.items) ? payload.items : parseItemsText(payload.items_text);
  return {
    ...payload,
    items: [...existingItems, ...importedItems],
    items_text: undefined,
  };
}

function formatDurationFromDates(start, end) {
  if (!start) return "-";
  const started = new Date(start).getTime();
  const finished = end ? new Date(end).getTime() : Date.now();
  if (!Number.isFinite(started) || !Number.isFinite(finished)) return "-";
  const seconds = Math.max(0, Math.round((finished - started) / 1000));
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  if (hours > 0) return `${hours}h ${minutes}m`;
  if (minutes > 0) return `${minutes}m ${seconds % 60}s`;
  return `${seconds}s`;
}

function markdownReport(data) {
  const lines = [];
  const counts = data.counts || {};
  lines.push(`# qcloop 托管报告: ${data.name || data.job_id}`);
  lines.push("");
  lines.push(`- 批次 ID: ${data.job_id}`);
  lines.push(`- 状态: ${data.status}`);
  lines.push(`- 运行轮次: ${data.run_no}`);
  lines.push(`- 最大质检轮次: ${data.max_qc_rounds}`);
  lines.push(`- 执行器失败自动重试: ${data.max_executor_retries ?? 0}`);
  lines.push(`- 已耗时: ${formatDurationFromDates(data.created_at, data.finished_at)}`);
  lines.push("");
  lines.push("| 指标 | 数量 |");
  lines.push("| --- | ---: |");
  for (const key of ["total", "success", "failed", "exhausted", "awaiting_confirmation", "running", "pending", "canceled"]) {
    lines.push(`| ${key} | ${counts[key] || 0} |`);
  }
  if ((data.awaiting_confirmations || []).length > 0) {
    lines.push("");
    lines.push("## 待确认");
    for (const item of data.awaiting_confirmations) {
      lines.push(`- ${item.item_id}: ${item.question || item.item_value}`);
    }
  }
  if ((data.problems || []).length > 0) {
    lines.push("");
    lines.push("## 问题项");
    for (const item of data.problems) {
      lines.push(`- ${item.item_id} [${item.status}] ${item.item_value}`);
      if (item.last_feedback) lines.push(`  - 质检反馈: ${item.last_feedback.replace(/\n/g, " ")}`);
      if (item.last_stderr) lines.push(`  - stderr: ${item.last_stderr.replace(/\n/g, " ")}`);
      if (item.confirmation_question) lines.push(`  - 待确认: ${item.confirmation_question.replace(/\n/g, " ")}`);
    }
  }
  return `${lines.join("\n")}\n`;
}

async function loadJobAndItems(baseUrl, jobId) {
  const job = await request("GET", baseUrl, `/api/jobs/${quote(jobId)}`);
  const items = await request("GET", baseUrl, `/api/items/?job_id=${quote(jobId)}`);
  return { job, items: items || [] };
}

async function cmdDoctor(baseUrl) {
  const candidates = [baseUrl];
  if (baseUrl !== API_FALLBACK_BASE_URL) candidates.push(API_FALLBACK_BASE_URL);
  const checks = [];
  let selected = null;
  for (const candidate of candidates) {
    const result = { base_url: candidate };
    try {
      const guide = await readText(candidate, "/llm-full.txt");
      result.llm_full = { ok: true, bytes: guide.length };
    } catch (error) {
      result.llm_full = { ok: false, error_code: error.code || "ERROR", error_message: error.message };
    }
    try {
      const jobs = await request("GET", candidate, "/api/jobs");
      result.api_jobs = { ok: true, count: (jobs || []).length };
    } catch (error) {
      result.api_jobs = { ok: false, error_code: error.code || "ERROR", error_message: error.message };
    }
    checks.push(result);
    if (result.api_jobs.ok) {
      selected = candidate;
      break;
    }
  }
  emit(envelope("doctor", { selected_base_url: selected, checks }, { ready: selected !== null }));
  return selected ? 0 : 2;
}

async function cmdGuide(baseUrl, args) {
  const full = takeFlag(args, "--full");
  const raw = takeFlag(args, "--raw");
  const path = full ? "/llm-full.txt" : "/llm.txt";
  const text = await readText(baseUrl, path);
  if (raw) {
    process.stdout.write(text);
    if (!text.endsWith("\n")) process.stdout.write("\n");
    return 0;
  }
  emit(envelope("guide", { path, bytes: text.length, text }));
  return 0;
}

async function cmdJob(baseUrl, args) {
  if (args.length === 0 || ["--help", "-h"].includes(args[0])) {
    console.log(jobHelp());
    return 0;
  }
  const sub = args.shift();
  if (sub === "list") {
    const status = takeOption(args, "--status", undefined);
    const limit = parseIntOption(args, "--limit", undefined);
    let jobs = (await request("GET", baseUrl, "/api/jobs")) || [];
    if (status) jobs = jobs.filter((job) => job.status === status);
    if (limit !== undefined) jobs = jobs.slice(0, limit);
    emit(envelope("job list", jobs, { count: jobs.length }));
    return 0;
  }
  if (sub === "create") {
    const file = takeOption(args, "--file", undefined);
    if (!file) throw new CliError("INVALID_ARGUMENT", "job create 需要 --file <payload.json>", { exitCode: 64 });
    const run = takeFlag(args, "--run");
    const mode = takeOption(args, "--mode", "auto");
    const importedItems = buildImportedItems(args);
    const payload = mergeImportedItems(JSON.parse(fs.readFileSync(file, "utf8")), importedItems);
    const job = await request("POST", baseUrl, "/api/jobs", payload);
    let runResult = null;
    if (run) runResult = await request("POST", baseUrl, "/api/jobs/run", { job_id: job.id, mode });
    emit(envelope("job create", job, { run_result: runResult, imported_item_count: importedItems.length, dashboard_url: `${baseUrl}/?job_id=${job.id}` }));
    return 0;
  }
  if (sub === "run") {
    const jobId = args.shift();
    if (!jobId) throw new CliError("INVALID_ARGUMENT", "job run 需要 <job_id>", { exitCode: 64 });
    const mode = takeOption(args, "--mode", "auto");
    const result = await request("POST", baseUrl, "/api/jobs/run", { job_id: jobId, mode });
    emit(envelope("job run", result, { job_id: jobId, mode }));
    return 0;
  }
  if (sub === "pause" || sub === "resume") {
    const jobId = args.shift();
    if (!jobId) throw new CliError("INVALID_ARGUMENT", `job ${sub} 需要 <job_id>`, { exitCode: 64 });
    const result = await request("POST", baseUrl, `/api/jobs/${sub}`, { job_id: jobId });
    emit(envelope(`job ${sub}`, result, { job_id: jobId }));
    return 0;
  }
  if (sub === "cancel") {
    const jobId = args.shift();
    if (!jobId) throw new CliError("INVALID_ARGUMENT", "job cancel 需要 <job_id>", { exitCode: 64 });
    const reason = takeOption(args, "--reason", "外层 AI 取消批处理");
    const result = await request("POST", baseUrl, "/api/jobs/cancel", { job_id: jobId, reason });
    emit(envelope("job cancel", result, { job_id: jobId }));
    return 0;
  }
  if (sub === "status") {
    const jobId = args.shift();
    if (!jobId) throw new CliError("INVALID_ARGUMENT", "job status 需要 <job_id>", { exitCode: 64 });
    const includeItems = takeFlag(args, "--include-items");
    const problemLimit = parseIntOption(args, "--problem-limit", 20);
    const { job, items } = await loadJobAndItems(baseUrl, jobId);
    const data = summarizeJob(job, items, problemLimit);
    if (includeItems) data.items = items;
    emit(envelope("job status", data));
    return 0;
  }
  if (sub === "wait") {
    const jobId = args.shift();
    if (!jobId) throw new CliError("INVALID_ARGUMENT", "job wait 需要 <job_id>", { exitCode: 64 });
    const timeout = parseIntOption(args, "--timeout", 1800);
    const interval = parseFloatOption(args, "--interval", 5);
    const problemLimit = parseIntOption(args, "--problem-limit", 20);
    const deadline = Date.now() + timeout * 1000;
    while (true) {
      const { job, items } = await loadJobAndItems(baseUrl, jobId);
      const data = summarizeJob(job, items, problemLimit);
      if ((data.counts.awaiting_confirmation || 0) > 0) {
        emit(envelope("job wait", data, { terminal: false, needs_confirmation: true, dashboard_url: `${baseUrl}/?job_id=${jobId}` }));
        return 3;
      }
      if (TERMINAL_STATUSES.has(job.status)) {
        emit(envelope("job wait", data, { terminal: true, dashboard_url: `${baseUrl}/?job_id=${jobId}` }));
        return job.status === "completed" ? 0 : 2;
      }
      if (Date.now() >= deadline) {
        emit(envelope("job wait", data, { terminal: false, timed_out: true, dashboard_url: `${baseUrl}/?job_id=${jobId}` }));
        return 124;
      }
      await new Promise((resolve) => setTimeout(resolve, interval * 1000));
    }
  }
  if (sub === "report") {
    const jobId = args.shift();
    if (!jobId) throw new CliError("INVALID_ARGUMENT", "job report 需要 <job_id>", { exitCode: 64 });
    const format = takeOption(args, "--format", "json");
    const problemLimit = parseIntOption(args, "--problem-limit", 50);
    const { job, items } = await loadJobAndItems(baseUrl, jobId);
    const data = summarizeJob(job, items, problemLimit);
    if (format === "markdown") {
      emit(envelope("job report", { ...data, markdown: markdownReport(data) }, { dashboard_url: `${baseUrl}/?job_id=${jobId}` }));
      return 0;
    }
    if (format !== "json") {
      throw new CliError("INVALID_ARGUMENT", "--format 只能是 json 或 markdown", { exitCode: 64 });
    }
    emit(envelope("job report", data, { dashboard_url: `${baseUrl}/?job_id=${jobId}` }));
    return 0;
  }
  throw new CliError("UNKNOWN_COMMAND", `未知 job 子命令: ${sub}`, { hint: jobHelp(), exitCode: 64 });
}

async function cmdItem(baseUrl, args) {
  if (args.length === 0 || ["--help", "-h"].includes(args[0])) {
    console.log(itemHelp());
    return 0;
  }
  const sub = args.shift();
  if (sub === "answer") {
    const itemId = args.shift();
    if (!itemId) throw new CliError("INVALID_ARGUMENT", "item answer 需要 <item_id>", { exitCode: 64 });
    const answer = takeOption(args, "--answer", undefined);
    if (!answer) throw new CliError("INVALID_ARGUMENT", "item answer 需要 --answer <text>", { exitCode: 64 });
    const resume = takeFlag(args, "--resume");
    const result = await request("POST", baseUrl, "/api/items/answer", { item_id: itemId, answer, resume });
    emit(envelope("item answer", result, { item_id: itemId, resumed: resume }));
    return 0;
  }
  if (sub === "retry") {
    const itemId = args.shift();
    if (!itemId) throw new CliError("INVALID_ARGUMENT", "item retry 需要 <item_id>", { exitCode: 64 });
    const result = await request("POST", baseUrl, "/api/items/retry", { item_id: itemId });
    emit(envelope("item retry", result, { item_id: itemId }));
    return 0;
  }
  if (sub === "cancel") {
    const itemId = args.shift();
    if (!itemId) throw new CliError("INVALID_ARGUMENT", "item cancel 需要 <item_id>", { exitCode: 64 });
    const reason = takeOption(args, "--reason", "外层 AI 取消单个 item");
    const result = await request("POST", baseUrl, "/api/items/cancel", { item_id: itemId, reason });
    emit(envelope("item cancel", result, { item_id: itemId }));
    return 0;
  }
  throw new CliError("UNKNOWN_COMMAND", `未知 item 子命令: ${sub}`, { hint: itemHelp(), exitCode: 64 });
}

async function cmdTemplate(baseUrl, args) {
  if (args.length === 0 || ["--help", "-h"].includes(args[0])) {
    console.log(templateHelp());
    return 0;
  }
  const sub = args.shift();
  if (sub === "list") {
    const templates = (await request("GET", baseUrl, "/api/templates")) || [];
    emit(envelope("template list", templates, { count: templates.length }));
    return 0;
  }
  if (sub === "show") {
    const templateId = args.shift();
    if (!templateId) throw new CliError("INVALID_ARGUMENT", "template show 需要 <template_id>", { exitCode: 64 });
    const template = await request("GET", baseUrl, `/api/templates/${quote(templateId)}`);
    emit(envelope("template show", template));
    return 0;
  }
  if (sub === "create") {
    const file = takeOption(args, "--file", undefined);
    if (!file) throw new CliError("INVALID_ARGUMENT", "template create 需要 --file <template.json>", { exitCode: 64 });
    const payload = JSON.parse(fs.readFileSync(file, "utf8"));
    const template = await request("POST", baseUrl, "/api/templates", payload);
    emit(envelope("template create", template));
    return 0;
  }
  if (sub === "update") {
    const templateId = args.shift();
    if (!templateId) throw new CliError("INVALID_ARGUMENT", "template update 需要 <template_id>", { exitCode: 64 });
    const file = takeOption(args, "--file", undefined);
    if (!file) throw new CliError("INVALID_ARGUMENT", "template update 需要 --file <template.json>", { exitCode: 64 });
    const payload = JSON.parse(fs.readFileSync(file, "utf8"));
    const template = await request("PUT", baseUrl, `/api/templates/${quote(templateId)}`, payload);
    emit(envelope("template update", template));
    return 0;
  }
  if (sub === "delete") {
    const templateId = args.shift();
    if (!templateId) throw new CliError("INVALID_ARGUMENT", "template delete 需要 <template_id>", { exitCode: 64 });
    const result = await request("DELETE", baseUrl, `/api/templates/${quote(templateId)}`);
    emit(envelope("template delete", result, { template_id: templateId }));
    return 0;
  }
  throw new CliError("UNKNOWN_COMMAND", `未知 template 子命令: ${sub}`, { hint: templateHelp(), exitCode: 64 });
}

async function cmdQueue(baseUrl, args) {
  if (args.length === 0 || ["--help", "-h"].includes(args[0])) {
    console.log(queueHelp());
    return 0;
  }
  const sub = args.shift();
  if (sub === "metrics") {
    const metrics = await request("GET", baseUrl, "/api/queue/metrics");
    emit(envelope("queue metrics", metrics));
    return 0;
  }
  throw new CliError("UNKNOWN_COMMAND", `未知 queue 子命令: ${sub}`, { hint: queueHelp(), exitCode: 64 });
}

function cmdSkill(args) {
  if (args.length === 0 || ["--help", "-h"].includes(args[0])) {
    console.log(skillHelp());
    return 0;
  }
  const sub = args.shift();
  if (sub === "list") {
    emit(envelope("skill list", SKILL_CATALOG, { count: SKILL_CATALOG.length }));
    return 0;
  }
  if (sub === "show") {
    const name = args.shift();
    const found = SKILL_CATALOG.find((item) => item.name.toLowerCase() === String(name || "").toLowerCase());
    if (!found) throw new CliError("SKILL_NOT_FOUND", `未知技能: ${name}`, { hint: "运行 qcloop-skill skill list 查看可用项。", exitCode: 4 });
    emit(envelope("skill show", found));
    return 0;
  }
  throw new CliError("UNKNOWN_COMMAND", `未知 skill 子命令: ${sub}`, { hint: skillHelp(), exitCode: 64 });
}

async function cmdApi(baseUrl, args) {
  const method = (args.shift() || "").toUpperCase();
  const path = args.shift();
  if (!method || !path) throw new CliError("INVALID_ARGUMENT", "api 需要 <method> <path>", { exitCode: 64 });
  let body;
  const file = takeOption(args, "--file", undefined);
  const data = takeOption(args, "--data", undefined);
  if (file) body = JSON.parse(fs.readFileSync(file, "utf8"));
  if (data) body = JSON.parse(data);
  const result = await request(method, baseUrl, path, body);
  emit(envelope("api", result, { method, path }));
  return 0;
}

async function main() {
  const { args, help, baseUrl } = parseGlobalArgs(process.argv.slice(2));
  if (help || args.length === 0) {
    console.log(rootHelp());
    return 0;
  }
  const command = args.shift();
  switch (command) {
    case "doctor":
      return await cmdDoctor(baseUrl);
    case "guide":
      return await cmdGuide(baseUrl, args);
    case "job":
      return await cmdJob(baseUrl, args);
    case "item":
      return await cmdItem(baseUrl, args);
    case "template":
      return await cmdTemplate(baseUrl, args);
    case "queue":
      return await cmdQueue(baseUrl, args);
    case "skill":
      return cmdSkill(args);
    case "api":
      return await cmdApi(baseUrl, args);
    default:
      throw new CliError("UNKNOWN_COMMAND", `未知命令: ${command}`, { hint: rootHelp(), exitCode: 64 });
  }
}

main()
  .then((code) => process.exit(code))
  .catch((error) => {
    emitError(error);
    process.exit(error.exitCode || 1);
  });
