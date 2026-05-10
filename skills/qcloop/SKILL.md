---
name: qcloop
description: 通过 qcloop 本地 Web/API 服务驱动 AI 批量测试与质检闭环。适用于用户要求使用 qcloop、运行 qcloop 测试、创建或监控 qcloop 批次、把多个检查项拆成队列任务，或需要通过 worker/verifier/repair 多轮闭环验证当前任务的场景。
---

# qcloop

## 概览

把 qcloop 当作执行编排器使用：外层 AI agent 负责理解当前任务、拆分测试项、创建批次、启动运行、监控证据、必要时向人类确认并写回答案，最后汇报结果。qcloop 负责队列、执行器失败重试、返修轮次、issue ledger、待确认状态、单 item 重试/取消、模板复用、队列指标、终态判断和 Web 可视化。

## 工作流

1. 创建任务前先确认 qcloop 已运行：
   - 优先检查 `GET http://127.0.0.1:3000/llm-full.txt`。
   - API 兜底地址：`http://127.0.0.1:8080`。
   - 如果两个地址都不可访问，请让人类先打开 qcloop；不要把手动启动命令当作默认主路径。
2. 创建批次前先读取 `/llm-full.txt`。它是当前本机 qcloop 版本的运行契约。
3. 从对话和目标仓库状态推断当前任务，拆分成可以独立执行和验收的 `items`。
4. 使用 `POST /api/jobs` 创建批次，再用 `POST /api/jobs/run` 启动；文件型批量任务可用 CLI 的 `--items-dir` / `--glob` / `--git-diff` 自动导入 items。
5. 通过 `GET /api/jobs/:id` 和 `GET /api/items/?job_id=...` 监控，直到批次进入终态或出现 `awaiting_confirmation`。
6. 如果有 `awaiting_confirmation`，读取 `confirmation_question` / `last_error`，向人类提出一个确认问题；拿到答案后调用 `item answer ... --resume` 继续，不要让人类手动操作 UI。
7. 如果只是单个 item 需要重跑或跳过，用 `item retry` / `item cancel`；不要为了局部问题重建整个批次。
8. 长跑期间可用 `queue metrics` 判断 worker 是否活跃、pending/running/stale 是否异常；高频任务可用 `template` 命令复用配置。
9. 用 `job report` 汇报批次 ID、Web 地址、统计数字、失败/已耗尽/待确认/已取消证据，以及是否需要后续修复。

## 技能 CLI

优先使用 `qcloop-skill` CLI，不要手写易出错的 `curl`。它可以来自 npm 包 `@limecloud/qcloop-skill-cli`，也可以来自仓库内置脚本。它参考 Lime CLI 风格：默认输出结构化 JSON，提供 `doctor`、`job`、`skill` 和原始 `api` 逃生口。命令路径应相对本技能目录解析。

```bash
qcloop-skill doctor
qcloop-skill guide --full --raw
qcloop-skill job create --file /tmp/qcloop-job.json --run
qcloop-skill job create --file /tmp/qcloop-job.json --cwd "$PWD" --glob "docs/**/*.md" --run
qcloop-skill job wait <job_id> --timeout 1800
qcloop-skill job report <job_id> --format markdown
qcloop-skill item answer <item_id> --answer "允许继续，但不要提交" --resume
qcloop-skill item retry <item_id>
qcloop-skill item cancel <item_id> --reason "本项暂不处理"
qcloop-skill queue metrics
qcloop-skill template list

# 没有全局命令时使用仓库内置版本
skills/qcloop/bin/qcloop-skill doctor
skills/qcloop/bin/qcloop-skill guide --full --raw
skills/qcloop/bin/qcloop-skill job create --file /tmp/qcloop-job.json --run
skills/qcloop/bin/qcloop-skill job wait <job_id> --timeout 1800
skills/qcloop/bin/qcloop-skill job report <job_id> --format markdown
skills/qcloop/bin/qcloop-skill item answer <item_id> --answer "允许继续，但不要提交" --resume
skills/qcloop/bin/qcloop-skill queue metrics
```

如果 Web/API 地址不是 `http://127.0.0.1:3000`，设置 `QCLOOP_BASE_URL` 或传 `--base-url`。如果可执行包装脚本不可用，用同样参数运行 `python3 skills/qcloop/scripts/qcloop_cli.py ...`。

常用命令：

- `qcloop-skill doctor`：检查 `/llm-full.txt` 和 `/api/jobs`。
- `qcloop-skill job list --status failed --limit 20`：发现最近失败批次。
- `qcloop-skill job status <job_id> --include-items`：读取摘要和证据。
- `qcloop-skill job report <job_id> --format markdown`：生成睡前托管报告，适合长跑任务收尾。
- `qcloop-skill job create --file payload.json --glob "docs/**/*.md" --run`：由外层 AI 自动把文件集导入为结构化 items。
- `qcloop-skill item answer <item_id> --answer "..." --resume`：写回人类确认答案并恢复该 item。
- `qcloop-skill item retry <item_id>`：重试单个 item，保留历史记录并重新入队。
- `qcloop-skill item cancel <item_id> --reason "..."`：取消单个未完成 item。
- `qcloop-skill queue metrics`：读取队列指标，判断是否活跃、是否卡住。
- `qcloop-skill template list/show/create/update/delete`：管理常用批次模板。
- `qcloop-skill job run <job_id> --mode retry_unfinished`：重试失败/已耗尽项。
- `qcloop-skill job cancel <job_id> --reason "..."`：终止不再需要的批次，进入不可恢复终态。
- `qcloop-skill api GET /api/jobs`：只读原始 API 逃生口。

## 批次设计规则

- 每个 item 都要能独立执行和验收，可以是文件路径、测试场景、接口、UI 流程或性能用例。
- 需要自动返修时，`max_qc_rounds` 推荐 `3-5`。设为 `1` 只会执行 worker + verifier；质检失败会直接变成 `exhausted`。
- `max_executor_retries` 推荐 `1`，只用于本机 AI CLI 启动/进程类基础设施错误；不要用它掩盖目标测试失败。
- 只有当用户或环境明确要求时才显式设置 `executor_provider`：`codex`、`claude_code`、`gemini_cli` 或 `kiro_cli`。
- `prompt_template` 必须包含 `{{item}}`；verifier prompt 可使用 `{{stdout}}`、`{{stderr}}`、`{{exit_code}}`、`{{attempt_status}}`、`{{attempt_type}}`、`{{qc_history}}` 或 `{{issue_ledger}}`。
- verifier 正常输出 `{"pass": true|false, "feedback": "..."}`；遇到需要人确认的歧义/高风险动作时输出 `{"pass": false, "needs_confirmation": true, "question": "...", "feedback": "..."}`。
- 要求内层 worker 只运行最小相关验证；除非人类明确授权，不要执行 `git commit`、`git push` 或 `git reset`。

最小批次 payload 示例：

```json
{
  "name": "task-focused-smoke",
  "prompt_template": "针对 {{item}} 检查目标仓库，运行最小相关验证；如允许且必要则做最小修复，最后报告修改文件和验证结果。",
  "verifier_prompt_template": "评估 {{item}}。stdout={{stdout}} stderr={{stderr}} exit_code={{exit_code}} status={{attempt_status}}。只输出 JSON: {\"pass\": true|false, \"feedback\": \"...\"}。",
  "items_text": "case-or-path-1\n{\"name\":\"review doc\",\"target\":\"docs/a.md\"}",
  "max_qc_rounds": 3,
  "max_executor_retries": 1,
  "executor_provider": "codex"
}
```

## 结果语义

- `completed` 表示所有 item 都成功。
- `failed` 可以是终态：至少一个 item 是 `failed` 或 `exhausted`。
- `waiting_confirmation` 表示当前存在待确认 item；外层 AI 应提问并写回答案。
- `canceled` 表示批次已被外层 AI 或人类明确终止，不会继续运行。
- `awaiting_confirmation` 是 item 级状态，不是让人手动操作，而是让外层 AI 获取确认后继续。
- `exhausted` 表示达到最大质检轮次或 token 预算；要检查 verifier feedback 和最后一次 attempt 输出。
- 总结时必须引用 attempts 和 qc_rounds 里的证据，不要只看顶层状态。
