# Superpowers 工作流强制执行

**所有功能开发必须严格按以下流程，不得跳过任何步骤：**

```
1. brainstorming        → 需求分析 + 设计方案 + 写 spec
2. writing-plans        → 出实现计划
3. executing-plans      → 按计划逐步实现（inline 执行）
4. verification-before-completion → 测试 + 构建 + E2E + 代码审查
5. finishing-a-development-branch → 合并到 master、推送远端
```

**违反即停止：**

- ❌ 跳过 brainstorming 直接写代码 → 停止，先调用 Skill brainstorming
- ❌ 不写 spec 就开始实现 → 停止
- ❌ 完成后不做 verification 就合并 → 停止，先 verification
- ❌ 删除操作不先确认 → 停止，先 AskUserQuestion
- ❌ 不创建 feature 分支直接在 master 开发 → 停止

**触发时机：**

- 用户说"我要做"、"帮我实现"、"开发一个" → 立即调用 Skill: `superpowers:brainstorming`
- 功能代码写完 → 立即调用 Skill: `superpowers:verification-before-completion`
- 验证通过后 → 立即调用 Skill: `superpowers:finishing-a-development-branch`
