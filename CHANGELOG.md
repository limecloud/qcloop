# Changelog

## v0.6.0 - 2026-05-10

详见 [docs/release-notes/v0.6.0.md](docs/release-notes/v0.6.0.md)。

- 增强 AI 托管主路径:Skill CLI 支持目录 / glob / git diff 导入 items、`job report` 睡前托管报告、`item retry/cancel`、`job cancel`、`queue metrics` 和模板 CRUD。
- 增强队列可靠性:`max_executor_retries` 独立重试本机 AI CLI 启动/进程类错误,不占用质检轮次。
- 增强多轮质检:verifier prompt 支持 `{{qc_history}}` / `{{issue_ledger}}`,用于判断旧问题是否修复、是否仍出现新问题。
- 增强 Web 观察面:队列指标面板、批次模板面板、单 item 重试/取消入口和取消状态统计。

## v0.5.0 - 2026-05-10

- 发布 `@limecloud/qcloop-skill-cli` npm 包,供 Skills 和外层 AI agent 直接使用。
- 补齐 Skill 中文使用说明与 `llms.txt` / `llms-full.txt` AI 托管工作流。

## v0.4.0 - 2026-05-10

详见 [docs/release-notes/v0.4.0.md](docs/release-notes/v0.4.0.md)。

## v0.1.0 - 2026-05-10

详见 [docs/release-notes/v0.1.0.md](docs/release-notes/v0.1.0.md)。
