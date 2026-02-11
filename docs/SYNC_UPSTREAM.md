# 同步上游代码并维护个人分支 - 完整操作指南

本文档说明如何：
1. 同步上游主项目（Tencent/WeKnora）的最新代码
2. 在本地维护你的个性化修改
3. 推送到你自己的 GitHub 账号

---

## 项目信息

- **上游项目**：https://github.com/Tencent/WeKnora.git
- **你的 Fork**：https://github.com/mrsteed/WeKnora.git
- **当前分支**：dev
- **配置状态**：✓ 已完成

---

## 完整操作流程

### 第一步：Fork 主项目到你的 GitHub 账号（仅首次需要）

1. 访问上游项目：https://github.com/Tencent/WeKnora
2. 点击右上角 **Fork** 按钮
3. Fork 到你的 GitHub 账号（已完成：`https://github.com/mrsteed/WeKnora`）

---

### 第二步：配置远端仓库（已完成 ✓）

```bash
# 进入项目目录
cd /home/xmkp/workspace/WeKnora

# 查看当前远端
git remote -v

# 重命名 origin 为 upstream（上游）
git remote rename origin upstream

# 添加你自己的 Fork 仓库为 origin
git remote add origin https://github.com/mrsteed/WeKnora.git

# 确认配置成功
git remote -v
# 当前配置：
# origin    https://github.com/mrsteed/WeKnora.git (fetch)
# origin    https://github.com/mrsteed/WeKnora.git (push)
# upstream  https://github.com/Tencent/WeKnora.git (fetch)
# upstream  https://github.com/Tencent/WeKnora.git (push)
```

---

### 第三步：创建你的个性化开发分支（已完成 ✓）

```bash
# 创建并切换到你的开发分支
git checkout -b dev

# 推送到远端
git push -u origin dev
```

> **当前分支**：dev（已创建并推送到远端）

---

### 第四步：日常开发与提交

```bash
# 修改代码后，查看状态
git status

# 添加修改
git add .

# 提交到本地
git commit -m "feat: 你的修改说明"

# 推送到你自己的 GitHub 仓库
git push origin dev
```

---

### 第五步：定期同步上游最新代码

#### 方式一：Rebase（推荐，历史更清晰）

```bash
# 1. 获取上游最新代码
git fetch upstream

# 2. 切换到你的开发分支
git checkout dev

# 3. 将你的提交重放到上游最新代码之上
git rebase upstream/main

# 4. 如果有冲突，解决后：
git add <冲突文件>
git rebase --continue

# 5. 推送到你自己的远端（需要强制推送）
git push -f origin dev
```

#### 方式二：Merge（不改写历史）

```bash
# 1. 获取上游最新代码
git fetch upstream

# 2. 切换到你的开发分支
git checkout dev

# 3. 合并上游代码
git merge upstream/main

# 4. 如果有冲突，解决后提交
git add <冲突文件>
git commit

# 5. 推送到你自己的远端
git push origin dev
```

---

### 第六步：同步你的 main 分支（可选）

```bash
# 切换到 main 分支
git checkout main

# 拉取上游最新代码
git pull upstream main

# 推送到你自己的远端
git push origin main
```

---

## 冲突处理详解

### 当 rebase 或 merge 时出现冲突：

```bash
# 1. 查看冲突文件
git status

# 2. 手动编辑冲突文件，解决冲突标记：
#    <<<<<<< HEAD
#    你的代码
#    =======
#    上游代码
#    >>>>>>> upstream/main

# 3. 解决后标记已解决
git add <冲突文件>

# 4a. 如果是 rebase，继续：
git rebase --continue

# 4b. 如果是 merge，提交：
git commit
```

### 如果想放弃当前操作：

```bash
# 放弃 rebase
git rebase --abort

# 放弃 merge
git merge --abort
```

---

## 完整工作流程示例

```bash
# === 初始配置（仅首次） ===
git remote rename origin upstream
git remote add origin https://github.com/YOUR_USERNAME/WeKnora.git
git checkout -b dev

# === 日常开发 ===
# 修改代码...
git add .
git commit -m "feat: 添加新功能"
git push origin dev

# === 定期同步上游（每周或每次上游更新时） ===
git fetch upstream
git checkout dev
git rebase upstream/main
# 解决冲突（如有）
git push -f origin dev

# === 同步 main 分支 ===
git checkout main
git pull upstream main
git push origin main
```

---

## 常用命令速查

| 操作 | 命令 |
|------|------|
| 查看远端配置 | `git remote -v` |
| 查看当前分支 | `git branch` |
| 切换分支 | `git checkout <分支名>` |
| 查看状态 | `git status` |
| 暂存未提交改动 | `git stash` |
| 恢复暂存改动 | `git stash pop` |
| 查看提交历史 | `git log --oneline --graph` |
| 撤销本地修改 | `git checkout -- <文件>` |
| 强制与远端同步 | `git reset --hard origin/dev` |

---

## 注意事项

1. **首次配置后记得修改文档中的 `YOUR_USERNAME`**
2. **强制推送（`-f`）仅用于你自己的分支，不要用于共享分支**
3. **定期同步上游可以减少冲突**
4. **重要改动建议先备份或打 tag**
5. **如果不确定，先 `git stash` 保存当前工作**

---

## 快速配置命令（复制执行）

```bash
# 1. 重命名远端（已完成）
git remote rename origin upstream

# 2. 添加你的 Fork（已完成）
git remote add origin https://github.com/mrsteed/WeKnora.git

# 3. 创建开发分支（已完成）
git checkout -b dev

# 4. 推送到你的远端（已完成）
git push -u origin dev

# 5. 同步上游最新代码（日常使用）
git fetch upstream
git rebase upstream/main
git push origin dev
```

---

**遇到问题？**
- 查看 Git 状态：`git status`
- 查看远端配置：`git remote -v`
- 查看分支：`git branch -vv`
- 需要帮助时，先 `git stash` 保存工作，然后寻求帮助
