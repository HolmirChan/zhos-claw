# Sprint 4

> 创建: 2026-05-27
> 目标版本: -
> 来源: BACKLOG.md（BL-006）
> 状态: 进行中

## 任务清单

### SB-001 · index.html 和 webmanifest 品牌更新 [ ]
- **来源**: BL-006
- **子任务**:
  - [ ] `index.html` — `<title>PicoClaw</title>` → `ZaiAgent`
  - [ ] `site.webmanifest` — `name`/`short_name` 更新为 ZaiAgent
  - [ ] 提交
- **阻塞**: -

### SB-002 · 组件内硬编码文字替换 [ ]
- **来源**: BL-006
- **子任务**:
  - [ ] `assistant-message.tsx:75` — `<span>PicoClaw</span>` → `ZaiAgent`
  - [ ] `config-sections.tsx:87` — placeholder `~/.picoclaw/workspace` → `~/.zaiagent/workspace`
  - [ ] `config-sections.tsx:299` — placeholder `/var/lib/picoclaw/evolution` → `/var/lib/zaiagent/evolution`
  - [ ] `mqtt-form.tsx:46,87` — `/picoclaw` 默认值/placeholder → `/zaiagent`
  - [ ] 提交
- **阻塞**: -

### SB-003 · i18n 文案品牌替换 [ ]
- **来源**: BL-006
- **子任务**:
  - [ ] `en.json` — 所有 "PicoClaw" → "ZaiAgent"（约 6 处）
  - [ ] `zh.json` — 同上
  - [ ] `pt-br.json` — 同上
  - [ ] 提交
- **阻塞**: -

### SB-004 · 移除外部文档链接 [ ]
- **来源**: BL-006
- **子任务**:
  - [ ] `app-header.tsx` — 删除 `docs.picoclaw.io` 链接（约第 275 行）
  - [ ] `channel-config-page.tsx` — 删除 `docs.picoclaw.io` 链接（约第 409-410 行）
  - [ ] 提交
- **阻塞**: -

### SB-005 · logo 图片替换 [ ]
- **来源**: BL-006
- **子任务**:
  - [ ] `public/logo_with_text.png` — 替换为 ZaiAgent 版本的 logo 图片（需用户提供新图片）
  - [ ] 提交
- **阻塞**: 需要用户提供新的 logo_with_text.png
