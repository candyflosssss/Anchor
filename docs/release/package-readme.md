# Anchor CLI 测试包

这是无界面的本机核心与 CLI，不是桌面安装器或完整 Agent 托管产品。
版本/来源见 BUILD-INFO.json；许可见 LICENSE 或 LEGAL-NOTICE.txt，依赖声明见 THIRD_PARTY_NOTICES.txt。带 LEGAL-NOTICE.txt 的包仅供内部评估，不作为已获授权的公开发行。

## 安装与使用

1. 下载对应系统/架构的包，用发行页的 SHA256SUMS 检查完整性，再解压。校验值应来自可信渠道，SHA-256 文件本身不是数字签名。
2. Linux 在解压目录执行 `./anchor version`；Windows PowerShell 执行 `.\anchor.exe version`。不需要 Go 开发环境，也不需要自行安装 SQLite 驱动。
3. Linux 示例：

   ```bash
   ./anchor init
   ./anchor serve
   ```

   保持 serve 终端运行，在同一解压目录另开终端：

   ```bash
   ./anchor context add local --local
   ./anchor context use local
   ./anchor status
   ```

   Windows 使用相同参数，把 `./anchor` 换成 `.\anchor.exe`；Windows 是否已真机验收以发行页说明为准。
4. 完整命令见随包的 CLI-COMMANDS.md；源码开发脚本不包含在包内，不需要执行其中的源码开发示例。

状态默认保存在用户配置目录下的 anchor 子目录，与程序目录分离；可通过 `--data-dir` 指定。管理/网关默认只监听 127.0.0.1:17861/17860。serve 前台运行，关闭终端可能停止服务，没有自动注册系统服务或自启动。

供应商密钥从 stdin 输入；授权令牌仅在显式 show-once 时返回。当前凭据依赖文件权限/Windows ACL，不是静态加密。不要把自己的 state.db、owner.token 或带秘密的日志打包发送给他人。

## 升级与卸载

- 退出正在运行的 serve。确认进程已停止后，备份**整个状态目录**，包括可能存在的 SQLite WAL/SHM 和凭据；备份本身含秘密，按同样权限保护。
- 保留旧程序包，解压新包，用相同数据目录运行。先查版本，再做初始化/状态与功能核对；不要通过删除数据库“解决”迁移失败。
- 当前没有自动升级或迁移命令。若未来升级涉及 schema，必须使用对应版本明确支持的迁移方案，不能假设旧程序一定能打开新数据库。
- 回退前先停新进程，保留失败现场，恢复匹配版本的程序和完整状态备份；不覆盖升级后产生的有效新数据而不确认损失。
- 卸载时停止服务并删除程序目录即可；用户状态保留。只有用户明确决定清除密钥/配置时才删除状态目录。

尚不支持 SSH、真实 Agent 接管/恢复、跨协议转换或桌面 UI；不要把测试包宣传成这些能力已经可用。
