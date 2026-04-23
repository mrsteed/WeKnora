# Admin Helper

这个目录用于保存“最小认证初始化”辅助脚本，方便在空 PostgreSQL 数据库里快速补齐基础认证表，并创建默认管理员账号。

包含文件：

- `weknora_admin_setup.sh`
  负责读取项目根目录 `.env` 的数据库与 AES 密钥配置，执行最小必要 schema 初始化，并创建或更新管理员账号。
- `weknora_admin_helper.go`
  负责复用当前后端逻辑中的关键算法：
  - `bcrypt.DefaultCost` 密码哈希
  - 租户 `api_key` 生成逻辑
  - `enc:v1:` 格式的 AES-GCM 加密

## 默认账号

- 邮箱：`admin@hlsa.com`
- 用户名：`admin`
- 密码：`a1234567.`

可以通过环境变量覆盖：

- `ADMIN_EMAIL`
- `ADMIN_USERNAME`
- `ADMIN_PASSWORD`

## 使用方式

在项目根目录执行：

```bash
bash scripts/admin_helper/weknora_admin_setup.sh
```

## 脚本行为

脚本会执行以下操作：

1. 读取 `.env` 中的 `DB_*`、`TENANT_AES_KEY`、`SYSTEM_AES_KEY`。
2. 直接执行：
   - `migrations/versioned/000000_init.up.sql`
   - `migrations/versioned/000001_agent.up.sql`
3. 额外补齐 `users.is_super_admin` 字段。
4. 若管理员用户不存在：
   - 创建或复用默认租户 `admin's Workspace`
   - 创建管理员账号
5. 若管理员用户已存在：
   - 更新用户名、密码哈希、激活状态、跨租户权限和超管标记
6. 最后输出数据库中的最终管理员记录。

## 为什么不直接用 `make migrate-up`

当前仓库 `migrations/versioned` 目录存在重复版本号文件，完整迁移流程可能会被阻塞。这个辅助脚本只初始化管理员登录所需的最小 schema，避免因为后续业务迁移冲突而无法完成首个管理员账号创建。

## 注意事项

- 该脚本面向“空库快速初始化管理员”场景，不替代完整业务迁移。
- 如果 `tenants.api_key` 列宽仍是早期的 `VARCHAR(64)`，脚本会自动回退为写入明文 `sk-...` 格式租户密钥，以兼容最小 schema；如果列宽足够，则优先写入 `enc:v1:` 加密值。
- 运行前请确认 `.env` 指向的是你希望初始化的目标数据库。