# qcloop 技能

本目录存放面向 AI agent 的 qcloop 技能包。它的目标不是替代 Web 面板，而是让 Codex / Claude Code / Gemini CLI / Kiro CLI 这类外层 agent 可以稳定地：读取 qcloop 运行说明、从目录/glob/git diff 导入 items、创建批次、启动队列、轮询状态、必要时向人类确认并写回答案、局部重试/取消 item、读取队列指标、复用模板、汇总报告或取消批次。

## 当前技能

- [`qcloop`](qcloop/SKILL.md)：通过 qcloop Web/API 驱动批量 QA loop。

## 技能 CLI

`qcloop` 技能内置一个参考 Lime CLI 风格的轻量 CLI，同时提供 npm 包 `@limecloud/qcloop-skill-cli`：

```bash
# npm 安装后
qcloop-skill doctor
qcloop-skill guide --full --raw
qcloop-skill job create --file /tmp/qcloop-job.json --run
qcloop-skill job create --file /tmp/qcloop-job.json --cwd "$PWD" --glob "docs/**/*.md" --run
qcloop-skill job wait <job_id> --timeout 1800
qcloop-skill job report <job_id> --format markdown
qcloop-skill item answer <item_id> --answer "允许继续，但不要提交" --resume
qcloop-skill item retry <item_id>
qcloop-skill queue metrics
qcloop-skill template list
qcloop-skill skill list

# 仓库内置版本
skills/qcloop/bin/qcloop-skill doctor
skills/qcloop/bin/qcloop-skill guide --full --raw
skills/qcloop/bin/qcloop-skill job create --file /tmp/qcloop-job.json --run
skills/qcloop/bin/qcloop-skill job wait <job_id> --timeout 1800
skills/qcloop/bin/qcloop-skill item answer <item_id> --answer "允许继续，但不要提交" --resume
skills/qcloop/bin/qcloop-skill queue metrics
skills/qcloop/bin/qcloop-skill skill list
```

默认连接 `http://127.0.0.1:3000`，也可以设置：

```bash
export QCLOOP_BASE_URL=http://127.0.0.1:3000
```

npm 发布入口见 [`packages/qcloop-skill-cli-npm`](../packages/qcloop-skill-cli-npm/README.md)。CLI 默认输出结构化 JSON。失败时 stderr 也输出 JSON error envelope，便于 AI agent 判断 `retryable`、`hint` 和下一步动作。
