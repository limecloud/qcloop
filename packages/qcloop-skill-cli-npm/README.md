# `@limecloud/qcloop-skill-cli`

qcloop 技能 JSON CLI，面向 Codex / Claude Code / Gemini CLI / Kiro CLI 等 AI agent。它封装本机 qcloop Web/API，让 agent 可以稳定执行：

```text
doctor -> guide -> job create/run -> job wait/status -> item answer -> retry
```

默认连接 `http://127.0.0.1:3000`，可以通过 `QCLOOP_BASE_URL` 或 `--base-url` 覆盖。

## 安装

```bash
npm install -g @limecloud/qcloop-skill-cli
```

也可以临时使用：

```bash
npx @limecloud/qcloop-skill-cli doctor
```

## 常用命令

```bash
qcloop-skill doctor
qcloop-skill guide --full --raw
qcloop-skill job list --limit 20
qcloop-skill job create --file /tmp/qcloop-job.json --run
qcloop-skill job wait <job_id> --timeout 1800
qcloop-skill job status <job_id> --include-items
qcloop-skill item answer <item_id> --answer "允许继续，但不要提交" --resume
qcloop-skill job run <job_id> --mode retry_unfinished
qcloop-skill skill list
qcloop-skill api GET /api/jobs
```

## JSON 约定

成功时 stdout 输出：

```json
{
  "ok": true,
  "command": "job status",
  "data": {}
}
```

失败时 stderr 输出：

```json
{
  "ok": false,
  "error_code": "CONNECTION_FAILED",
  "error_message": "...",
  "retryable": true,
  "hint": "先打开 qcloop 应用..."
}
```

`job status` / `job wait` 会汇总 `counts`，并展开失败、已耗尽或待确认 item 的最后一次 `stderr`、verifier `feedback` 和 `confirmation_question`。如果返回 `needs_confirmation=true`，外层 AI 应向人类提问，再用 `item answer ... --resume` 写回答案继续。

## 发布

```bash
cd packages/qcloop-skill-cli-npm
npm publish --access public
```
