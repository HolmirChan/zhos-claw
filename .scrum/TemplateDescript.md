# 文档模板

## 目录结构

```
.scrum/
  BACKLOG.md
  TemplateDescript.md
  sprint_NNN.md
  specs/     ← 设计文档（brainstorming 产出）
  plans/     ← 实现计划（writing-plans 产出）
```

## BACKLOG.md 整体结构

```markdown
# Backlog

> 当前迭代: sprint_NNN.md | 无

## 待规划

### BL-XXX · <标题>
- **意图**: <一句话描述用户想要什么>
- **方案方向**: <推荐的技术方案或设计思路>
- **设计文档**: .scrum/specs/YYYY-MM-DD-<topic>-design.md | -
- **实现计划**: .scrum/plans/YYYY-MM-DD-<topic>.md | -
- **验收标准**:
  - [ ] <可验证的条件>

## 已交付

### BL-XXX · <标题>
- **已完成于** sprint_NNN.md
```

## BL 条目模板

待规划:
```markdown
### BL-XXX · <标题>
- **意图**: <用户想要什么，为什么>
- **方案方向**: <技术方案简述>
- **设计文档**: .scrum/specs/YYYY-MM-DD-<topic>-design.md | -
- **实现计划**: .scrum/plans/YYYY-MM-DD-<topic>.md | -
- **验收标准**:
  - [ ] <条件1>
  - [ ] <条件2>
```

已交付:
```markdown
### BL-XXX · <标题>
- **已完成于** sprint_NNN.md
```

## Sprint 文件模板

```markdown
# Sprint N

> 创建: YYYY-MM-DD
> 完成: YYYY-MM-DD | -
> 目标版本: vX.Y.Z | -
> 来源: BACKLOG.md（BL-XXX, BL-YYY）
> 状态: 进行中 | 已完成

## 任务清单

### SB-XXX · <任务标题> [ ]
- **来源**: BL-XXX
- **设计文档**: .scrum/specs/YYYY-MM-DD-<topic>-design.md
- **实现计划**: .scrum/plans/YYYY-MM-DD-<topic>.md
- **子任务**:
  - [ ] <具体可执行的步骤>
- **阻塞**: <原因 | ->
```

## 状态标记

SB 条目: `[ ]` 待开始 / `[~]` 进行中 / `[x]` 已完成
子任务:   `[ ]` 待开始 / `[~]` 进行中 / `[x]` 已完成

## 编号规则

- BL: 全局递增，从 001 开始，不回收
- SB: Sprint 内递增，从 001 开始，每个 Sprint 独立

## 需求生命周期

```
用户说"讨论需求"
  → superpowers:brainstorming
  → 产出设计文档 → .scrum/specs/
  → 需求概要写入 BACKLOG「待规划」

用户说"启动迭代"
  → superpowers:writing-plans
  → 产出实现计划 → .scrum/plans/
  → 生成 sprint_NNN.md（引用 spec 和 plan 路径）

用户说"执行"
  → superpowers:subagent-driven-development 或 superpowers:executing-plans
  → 同步更新 sprint 状态

迭代关闭
  → 已完成条目移到「已交付」，补 已完成于 sprint_NNN.md
  → Sprint 补完成时间
  → 未完成项回写「待规划」
```
