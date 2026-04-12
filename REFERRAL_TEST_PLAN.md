# 邀请返佣系统 — 人工测试方案

> 测试环境：`http://127.0.0.1:8080`
> 本文档覆盖推荐返佣系统的全部功能，包括管理后台设置、用户邀请绑定、充值返佣、提现流程、退款逆向、以及 per-user 开关等。

---

## 目录

1. [测试前准备](#1-测试前准备)
2. [管理后台 — 返佣设置](#2-管理后台--返佣设置)
3. [用户注册 — 邀请码绑定](#3-用户注册--邀请码绑定)
4. [用户端 — 邀请中心](#4-用户端--邀请中心)
5. [管理后台 — 充值入账（触发返佣）](#5-管理后台--充值入账触发返佣)
6. [返佣结算（pending → available）](#6-返佣结算pending--available)
7. [用户端 — 提现流程](#7-用户端--提现流程)
8. [管理后台 — 提现审核](#8-管理后台--提现审核)
9. [佣金转余额](#9-佣金转余额)
10. [管理后台 — 退款逆向](#10-管理后台--退款逆向)
11. [管理后台 — 手动调账](#11-管理后台--手动调账)
12. [管理后台 — 关系管理](#12-管理后台--关系管理)
13. [Per-User 邀请开关](#13-per-user-邀请开关)
14. [边界与异常测试](#14-边界与异常测试)

---

## 1. 测试前准备

### 1.1 准备账号

| 账号 | 角色 | 用途 |
|------|------|------|
| `admin@sub2api.local` | 管理员 | 后台操作 |
| `inviter@test.com` | 普通用户 | 邀请人（上线）|
| `invitee@test.com` | 待注册 | 被邀请人（下线）|
| `invitee2@test.com` | 待注册 | 第二个被邀请人 |
| `noref@test.com` | 普通用户 | 未开启邀请的用户 |

### 1.2 注册管理员和邀请人

1. 打开 `http://127.0.0.1:8080`
2. 用管理员账号登录（首次启动时密码在 Docker 日志中）
   ```
   docker compose -f docker-compose.dev.yml logs sub2api | grep -i password
   ```
3. 注册 `inviter@test.com` 用户（或通过管理后台创建）

---

## 2. 管理后台 — 返佣设置

### TC-2.1 开启返佣总开关

**操作步骤：**
1. 管理员登录 → 左侧菜单「系统设置」
2. 找到「推广返佣」区域
3. 打开「启用推广返佣」开关
4. 设置以下参数：
   - 一级返佣：开启
   - 一级返佣比例：`0.10`（10%）
   - 返佣模式：`每笔充值` 或 `仅首次充值`
   - 结算延迟天数：`0`（方便测试，生产建议 7）
   - 允许手动输入邀请码：开启
   - 绑定时机：首次充值前（开启）
   - 结算货币：CNY
5. 打开「提现功能」开关
6. 设置提现参数：
   - 最低提现金额：`1`
   - 最高提现金额：`10000`
   - 每日提现限制：`3`
   - 提现手续费率：`0`
   - 提现固定手续费：`0`
   - 需要人工审核：开启
   - 支持的提现方式：勾选 `支付宝`、`微信`
7. 点击「保存」

**预期结果：**
- 保存成功提示
- 刷新页面后设置值保持不变

### TC-2.2 验证前端菜单显示

**操作步骤：**
1. 以 `inviter@test.com` 登录
2. 查看左侧菜单

**预期结果：**
- 左侧菜单出现「邀请中心」入口
- 仪表盘顶部出现邀请推广卡片

---

## 3. 用户注册 — 邀请码绑定

### TC-3.1 获取邀请码

**操作步骤：**
1. 以 `inviter@test.com` 登录
2. 进入「邀请中心」页面
3. 查看「我的推广码」

**预期结果：**
- 显示一个 8 位大写字母+数字的推广码（如 `A3BK7NP2`）
- 有复制按钮可以复制

### TC-3.2 注册时绑定邀请码（手动输入）

**操作步骤：**
1. 退出当前登录
2. 点击「注册」
3. 填写邮箱 `invitee@test.com`、密码
4. 在「邀请码」输入框中填入 inviter 的推广码
5. 点击注册

**预期结果：**
- 注册成功
- 登录后进入「邀请中心」→ 显示已绑定关系
- 显示推荐人信息（邮箱掩码）

### TC-3.3 验证邀请码（无效码）

**操作步骤：**
1. 退出登录 → 注册页面
2. 在邀请码输入框输入 `XXXXXXXX`（无效码）

**预期结果：**
- 实时验证提示邀请码无效
- 注册时报错「referral code not found」

### TC-3.4 注册时绑定（通过链接）

**操作步骤：**
1. 复制邀请链接（邀请中心页面通常有链接复制功能）
2. 在新的浏览器/无痕窗口打开链接
3. 填写 `invitee2@test.com` 注册

**预期结果：**
- 邀请码自动填入
- 注册成功后自动绑定关系

### TC-3.5 禁止自邀请

**操作步骤：**
1. 以 `inviter@test.com` 登录
2. 进入「邀请中心」→ 复制自己的推广码
3. 退出 → 尝试用自己的邀请码注册新号
4. 或者：在邀请中心绑定自己的推广码

**预期结果：**
- 报错：不能绑定自己的邀请码

### TC-3.6 禁止重复绑定

**操作步骤：**
1. 以 `invitee@test.com` 登录（已绑定 inviter）
2. 进入「邀请中心」→ 尝试输入另一个邀请码绑定

**预期结果：**
- 页面显示已绑定信息，`can_bind = false`
- 无法再次绑定

---

## 4. 用户端 — 邀请中心

### TC-4.1 邀请概览

**操作步骤：**
1. 以 `inviter@test.com` 登录
2. 进入「邀请中心」

**预期结果：**
- 显示累计已结算佣金：￥0.00
- 可提现佣金：￥0.00
- 处理中佣金：￥0.00
- 已提现佣金：￥0.00
- 邀请用户数：显示已邀请的人数（如 1-2 人）

### TC-4.2 查看邀请人列表

**操作步骤：**
1. 在邀请中心页面查看「邀请人列表」tab

**预期结果：**
- 列出 `invitee@test.com`、`invitee2@test.com`
- 显示邮箱、绑定时间、来源、充值总额、佣金总额

### TC-4.3 查看流水（空状态）

**操作步骤：**
1. 在邀请中心页面查看「流水明细」tab

**预期结果：**
- 空列表或提示暂无流水

---

## 5. 管理后台 — 充值入账（触发返佣）

### TC-5.1 为被邀请人充值

**操作步骤：**
1. 管理员登录
2. 进入「推广返佣管理」→ 或使用 API 直接调用
3. 使用以下 API 创建充值订单（用 curl 或 Postman）：

```bash
# 先获取管理员 token
TOKEN="<管理员JWT>"

# 为 invitee 充值 100 元
curl -X POST http://127.0.0.1:8080/api/v1/admin/recharge-orders/credit \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": <invitee的用户ID>,
    "external_order_id": "TEST-ORDER-001",
    "provider": "manual_test",
    "currency": "CNY",
    "paid_amount": 100,
    "credited_balance_amount": 100,
    "gross_amount": 100
  }'
```

> **如何获取用户ID**：管理后台 → 用户管理 → 找到 invitee 用户，ID 在列表中显示

**预期结果：**
- 返回成功，包含 `recharge_order` 和 `commission_rewards`
- `commission_rewards` 数组中有 1 条记录：
  - `user_id` = inviter 的 ID
  - `reward_amount` = 10.0（100 * 10%）
  - `status` = `available`（因为结算延迟设为 0 天）

### TC-5.2 验证邀请人佣金更新

**操作步骤：**
1. 以 `inviter@test.com` 登录
2. 进入「邀请中心」

**预期结果：**
- 可提现佣金：￥10.00
- 累计已结算佣金：￥10.00
- 流水明细中有两条记录：
  - `reward_pending_credit` +10（pending 入账）
  - `reward_pending_to_available` -10/+10（pending → available 结转）

### TC-5.3 幂等性验证

**操作步骤：**
1. 用相同的 `external_order_id` 和 `provider` 再次调用充值 API

**预期结果：**
- 返回成功，但返回的是之前创建的同一订单
- 不产生新的 reward
- inviter 的佣金仍然是 ￥10.00

### TC-5.4 第二笔充值

**操作步骤：**
1. 用不同的 `external_order_id` 为 invitee 再充值 200 元
```bash
curl -X POST http://127.0.0.1:8080/api/v1/admin/recharge-orders/credit \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": <invitee的用户ID>,
    "external_order_id": "TEST-ORDER-002",
    "provider": "manual_test",
    "currency": "CNY",
    "paid_amount": 200,
    "credited_balance_amount": 200,
    "gross_amount": 200
  }'
```

**预期结果（返佣模式=每笔充值时）：**
- 新增 reward：￥20.00
- inviter 可提现佣金变为 ￥30.00

**预期结果（返佣模式=仅首次充值时）：**
- 不产生新 reward
- inviter 佣金仍为 ￥10.00

---

## 6. 返佣结算（pending → available）

### TC-6.1 延迟结算验证

**操作步骤：**
1. 管理后台 → 系统设置 → 将结算延迟改为 `1` 天
2. 为 invitee 创建新的充值订单
3. 查看 inviter 的佣金

**预期结果：**
- 新 reward 状态为 `pending`
- 处理中佣金增加
- 可提现佣金不变
- reward 的 `available_at` 为充值时间 + 1 天

### TC-6.2 结算触发

**操作步骤：**
1. 等待延迟时间过后（或将延迟改回 0）
2. 刷新 inviter 的邀请中心页面（触发 `SettlePendingRewards`）

**预期结果：**
- pending reward 变为 available
- 可提现佣金更新

---

## 7. 用户端 — 提现流程

### TC-7.1 添加收款账户

**操作步骤：**
1. 以 `inviter@test.com` 登录
2. 进入「邀请中心」→「收款账户」
3. 点击「添加收款账户」
4. 选择收款方式：支付宝
5. 填写：
   - 姓名：张三
   - 账号：zhangsan@alipay.com
6. 保存

**预期结果：**
- 账户创建成功
- 列表中显示：支付宝 - 张三 - zhan****@alipay.com（掩码）
- 标记为默认账户

### TC-7.2 修改收款账户（7天限制）

**操作步骤：**
1. 立即尝试修改刚添加的账户

**预期结果：**
- 报错：收款账户每 7 天只能修改一次

### TC-7.3 发起提现

**操作步骤：**
1. 进入「邀请中心」→「提现」
2. 输入提现金额：10
3. 选择收款方式：支付宝
4. 选择收款账户
5. 提交

**预期结果：**
- 提现申请创建成功
- 状态为「待审核」
- 可提现佣金减少 ￥10
- 流水明细新增冻结记录（available -10, frozen +10）

### TC-7.4 提现金额校验

**操作步骤：**
1. 尝试提现 ￥0.5（低于最低限额 ￥1）
2. 尝试提现超过可提现余额的金额

**预期结果：**
- ￥0.5：报错「提现金额无效」
- 超额：报错「可提现余额不足」

### TC-7.5 每日提现限制

**操作步骤：**
1. 连续发起 3 次提现（每次 ￥1）
2. 第 4 次提现

**预期结果：**
- 前 3 次成功
- 第 4 次报错「每日提现限制已达上限」

---

## 8. 管理后台 — 提现审核

### TC-8.1 查看待审核提现

**操作步骤：**
1. 管理员登录 → 「推广返佣管理」→「提现管理」（或侧边栏对应入口）
2. 查看提现列表

**预期结果：**
- 列表中显示 inviter 的提现申请
- 状态：待审核
- 显示金额、手续费、实际到账金额
- 显示收款方式和账户快照

### TC-8.2 审核通过

**操作步骤：**
1. 选择一笔提现 → 点击「审核通过」
2. 可选填写备注

**预期结果：**
- 状态变为「已审核」
- 显示审核人和审核时间

### TC-8.3 标记已打款

**操作步骤：**
1. 对已审核的提现 → 点击「标记已打款」

**预期结果：**
- 状态变为「已打款」
- inviter 的已提现佣金增加
- 流水明细：frozen -10, settled +10

### TC-8.4 审核拒绝

**操作步骤：**
1. 对一笔待审核的提现 → 点击「拒绝」
2. 填写拒绝原因（必填）

**预期结果：**
- 状态变为「已拒绝」
- inviter 的可提现佣金恢复（frozen → available）
- 显示拒绝原因

### TC-8.5 拒绝后重新提现

**操作步骤：**
1. 以 inviter 登录
2. 验证可提现余额已恢复
3. 重新发起提现

**预期结果：**
- 余额正确恢复
- 可以重新提现

---

## 9. 佣金转余额

### TC-9.1 佣金转余额成功

**操作步骤：**
1. 以 `inviter@test.com` 登录
2. 邀请中心 → 点击「转为余额」（或调用 API）
3. 输入金额（如 ￥5）

```bash
# API 方式
curl -X POST http://127.0.0.1:8080/api/v1/user/referral/convert-to-credit \
  -H "Authorization: Bearer $INVITER_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"amount": 5}'
```

**预期结果：**
- 转换成功
- 可提现佣金减少 ￥5
- 用户余额增加 ￥5
- 提现记录中新增一条 `credit_conversion` 类型的记录，状态为已完成
- 流水明细中有完整的 4 步 ledger 记录：
  - available -5
  - frozen +5
  - frozen -5
  - settled +5

### TC-9.2 佣金转余额 — 余额不足

**操作步骤：**
1. 尝试转换超过可提现余额的金额

**预期结果：**
- 报错「可提现余额不足」

---

## 10. 管理后台 — 退款逆向

### TC-10.1 全额退款 — 扣回 pending 佣金

**前置条件：** 结算延迟设为 1 天，invitee 有一笔未结算的充值

**操作步骤（API）：**
```bash
# 注意：退款 API 需根据实际路由调整，此处模拟退款逻辑
# 如果系统有退款 API，直接调用；否则可以通过代码测试
```

**预期结果：**
- pending 状态的 reward 被 reverse
- pending 佣金减少
- 流水明细记录：`refund_reverse` -金额（pending bucket）

### TC-10.2 全额退款 — 扣回 available 佣金

**前置条件：** invitee 有一笔已结算的充值（reward 为 available）

**预期结果：**
- reward 状态变为 `reversed`
- available 佣金减少
- 流水记录：`refund_reverse` -金额（available bucket）

### TC-10.3 已提现佣金的退款 — negative carry

**前置条件：** invitee 有一笔充值，佣金已提现（reward 为 paid）

**操作步骤：**
1. 管理后台设置 → 确认「退款逆向-负值结转」开启
2. 执行退款

**预期结果：**
- reward 状态变为 `reversed` 或 `partially_reversed`
- 流水记录：`negative_carry` -金额（available bucket）
- inviter 的可提现佣金变为负数（允许的行为，会从后续佣金中扣除）

---

## 11. 管理后台 — 手动调账

### TC-11.1 增加佣金

**操作步骤：**
1. 管理员登录 → 推广返佣管理 → 佣金奖励列表
2. 找到 inviter 的一条 reward
3. 点击「调账」→ 输入金额 `+5`，备注「补偿」

```bash
# API 方式
curl -X POST http://127.0.0.1:8080/api/v1/admin/referral/commission-adjustments \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"reward_id": <reward_id>, "amount": 5, "remark": "compensation"}'
```

**预期结果：**
- 流水明细新增 `admin_add` +5（available bucket）
- inviter 可提现佣金增加 ￥5

### TC-11.2 扣减佣金

**操作步骤：**
1. 调账金额填 `-3`

**预期结果：**
- 流水明细新增 `admin_subtract` -3（available bucket）
- inviter 可提现佣金减少 ￥3

### TC-11.3 扣减超过可用余额

**操作步骤：**
1. 调账金额填一个超过该 reward 可用余额的负数

**预期结果：**
- 报错「可提现余额不足」

---

## 12. 管理后台 — 关系管理

### TC-12.1 查看邀请关系列表

**操作步骤：**
1. 管理员登录 → 推广返佣管理 → 邀请关系

**预期结果：**
- 列出所有绑定关系
- 每行显示：被邀请人、邀请人、绑定方式、绑定时间

### TC-12.2 修改邀请关系

**操作步骤：**
1. 找到 invitee 的关系记录
2. 点击修改 → 输入另一个用户的邀请码
3. 填写原因：「用户申请转移」

**预期结果：**
- 关系更新成功
- invitee 的邀请人变更为新的用户
- 关系变更历史中记录了此次操作

### TC-12.3 查看关系变更历史

**操作步骤：**
1. 推广返佣管理 → 关系变更记录
2. 筛选 invitee 的 user_id

**预期结果：**
- 列出所有变更记录
- 包含旧邀请人、新邀请人、操作人、原因、时间

### TC-12.4 查看邀请树

**操作步骤：**
1. 推广返佣管理 → 输入 inviter 的 user_id → 查看邀请树

**预期结果：**
- 树形展示 inviter → invitee、invitee2
- 每个节点显示邀请人数、佣金统计

### TC-12.5 管理后台总览

**操作步骤：**
1. 推广返佣管理 → 总览/Dashboard

**预期结果：**
- 显示全局统计：总账户数、绑定用户数
- 显示佣金汇总：待结算、可提现、冻结中、已提现
- 待处理提现数量和金额
- 近 7 天趋势图
- Top 10 排行榜

---

## 13. Per-User 邀请开关

### TC-13.1 全局关闭后菜单隐藏

**操作步骤：**
1. 管理后台 → 系统设置 → 关闭「启用推广返佣」
2. 以 `inviter@test.com` 登录

**预期结果：**
- 左侧菜单不显示「邀请中心」
- 仪表盘不显示邀请推广卡片

### TC-13.2 对指定用户开启邀请

**操作步骤：**
1. 管理后台 → 用户管理 → 找到 `inviter@test.com`
2. 点击编辑 → 打开「邀请功能」开关 → 保存
3. 以 `inviter@test.com` 重新登录（或刷新页面）

**预期结果：**
- 左侧菜单重新出现「邀请中心」
- 仪表盘出现邀请推广卡片
- 邀请中心功能正常（可以看到推广码、邀请记录等）

### TC-13.3 其他用户仍不可见

**操作步骤：**
1. 以 `noref@test.com`（未开启邀请功能的用户）登录

**预期结果：**
- 左侧菜单不显示「邀请中心」
- 直接访问 `/referral` 路由 → 显示无权限或空状态

### TC-13.4 Per-User 用户的邀请码可被使用

**操作步骤：**
1. 确认全局邀请开关仍然关闭
2. 复制 inviter 的推广码（inviter 已单独开启邀请功能）
3. 退出 → 注册新用户 `newuser@test.com`，填入 inviter 的推广码

**预期结果：**
- 注册成功
- 新用户自动绑定 inviter 作为邀请人
- 验证码验证通过（`ValidateReferralCode` 检查 code owner 的权限）

### TC-13.5 未开启用户的邀请码不可使用

**操作步骤：**
1. 确认全局邀请开关关闭
2. 确认 `noref@test.com` 的邀请功能也是关闭的
3. 通过 API 获取 noref 的推广码（EnsureDefaultCode 仍然会生成）
4. 尝试用 noref 的推广码注册新用户

**预期结果：**
- 注册时报错：referral program is disabled
- 邀请码验证失败

### TC-13.6 Per-User 用户的提现功能

**前置条件：** 全局关闭，inviter 单独开启，inviter 有可提现佣金

**操作步骤：**
1. 以 inviter 登录 → 邀请中心 → 发起提现

**预期结果：**
- 如果全局提现开关（`referral_withdraw_enabled`）也开启，提现正常
- 如果全局提现开关关闭，即使 per-user 开启了邀请功能，提现仍然被阻止（提现功能受全局 `referral_withdraw_enabled` 控制）

### TC-13.7 关闭 Per-User 开关

**操作步骤：**
1. 管理后台 → 用户管理 → 找到 inviter → 关闭「邀请功能」
2. 刷新 inviter 的页面

**预期结果：**
- 邀请中心菜单消失
- 已有的邀请关系和佣金数据不受影响（数据保留）

### TC-13.8 全局开启时 Per-User 字段无影响

**操作步骤：**
1. 管理后台 → 系统设置 → 重新开启「启用推广返佣」
2. 确认 `noref@test.com` 的 per-user 邀请功能是关闭的
3. 以 `noref@test.com` 登录

**预期结果：**
- 全局开启时，所有用户都能看到邀请中心（不管 per-user 设置）
- `noref@test.com` 也能正常使用邀请功能

---

## 14. 边界与异常测试

### TC-14.1 非 CNY 货币充值

```bash
curl -X POST http://127.0.0.1:8080/api/v1/admin/recharge-orders/credit \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": <user_id>,
    "external_order_id": "TEST-USD-001",
    "provider": "manual_test",
    "currency": "USD",
    "paid_amount": 100,
    "credited_balance_amount": 100
  }'
```

**预期结果：** 报错 `RECHARGE_ORDER_CURRENCY_INVALID`

### TC-14.2 金额为 0 或负数

```bash
curl -X POST http://127.0.0.1:8080/api/v1/admin/recharge-orders/credit \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": <user_id>,
    "external_order_id": "TEST-ZERO-001",
    "provider": "manual_test",
    "currency": "CNY",
    "paid_amount": 0,
    "credited_balance_amount": 0
  }'
```

**预期结果：** 报错 `RECHARGE_ORDER_AMOUNT_INVALID`

### TC-14.3 无邀请关系的用户充值

**操作步骤：**
1. 为没有绑定邀请码的 `noref@test.com` 充值

**预期结果：**
- 充值成功
- 不产生任何 commission reward
- 返回的 `commission_rewards` 为空数组

### TC-14.4 提现方式不在启用列表中

**操作步骤：**
1. 管理后台设置只启用「支付宝」
2. 用户提现时选择「银行卡」

**预期结果：** 报错 `COMMISSION_WITHDRAW_METHOD_INVALID`

### TC-14.5 调账金额超过 100 万

```bash
curl -X POST http://127.0.0.1:8080/api/v1/admin/referral/commission-adjustments \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"reward_id": 1, "amount": 1500000, "remark": "test"}'
```

**预期结果：** 报错「调账金额超过最大允许值」

### TC-14.6 不同用户提交同一外部订单号

```bash
# 先为用户A充值
curl -X POST ... -d '{"user_id": <A>, "external_order_id": "SAME-ORDER", ...}'
# 再为用户B用同一订单号充值
curl -X POST ... -d '{"user_id": <B>, "external_order_id": "SAME-ORDER", ...}'
```

**预期结果：** 第二次报错 `RECHARGE_ORDER_CONFLICT`

### TC-14.7 已充值用户绑定邀请码（bind_before_first_paid_only 开启时）

**前置条件：** 设置了「仅首次充值前可绑定」

**操作步骤：**
1. 注册 `late@test.com`（不填邀请码）
2. 为该用户充值
3. 尝试在邀请中心手动输入邀请码绑定

**预期结果：** 报错 `REFERRAL_BIND_AFTER_PAYMENT_NOT_ALLOWED`

### TC-14.8 未认证访问邀请接口

```bash
curl http://127.0.0.1:8080/api/v1/user/referral/overview
```

**预期结果：** 401 Unauthorized

### TC-14.9 普通用户访问管理接口

```bash
curl -X GET http://127.0.0.1:8080/api/v1/admin/referral/overview \
  -H "Authorization: Bearer $USER_TOKEN"
```

**预期结果：** 403 Forbidden

---

## 测试结果记录表

| 编号 | 测试项 | 状态 | 测试人 | 日期 | 备注 |
|------|--------|------|--------|------|------|
| TC-2.1 | 开启返佣总开关 | | | | |
| TC-2.2 | 前端菜单显示 | | | | |
| TC-3.1 | 获取邀请码 | | | | |
| TC-3.2 | 注册时绑定邀请码 | | | | |
| TC-3.3 | 无效邀请码验证 | | | | |
| TC-3.4 | 链接邀请注册 | | | | |
| TC-3.5 | 禁止自邀请 | | | | |
| TC-3.6 | 禁止重复绑定 | | | | |
| TC-4.1 | 邀请概览 | | | | |
| TC-4.2 | 邀请人列表 | | | | |
| TC-4.3 | 流水（空状态）| | | | |
| TC-5.1 | 充值触发返佣 | | | | |
| TC-5.2 | 佣金更新验证 | | | | |
| TC-5.3 | 幂等性验证 | | | | |
| TC-5.4 | 第二笔充值 | | | | |
| TC-6.1 | 延迟结算 | | | | |
| TC-6.2 | 结算触发 | | | | |
| TC-7.1 | 添加收款账户 | | | | |
| TC-7.2 | 7天修改限制 | | | | |
| TC-7.3 | 发起提现 | | | | |
| TC-7.4 | 提现金额校验 | | | | |
| TC-7.5 | 每日提现限制 | | | | |
| TC-8.1 | 查看待审核提现 | | | | |
| TC-8.2 | 审核通过 | | | | |
| TC-8.3 | 标记已打款 | | | | |
| TC-8.4 | 审核拒绝 | | | | |
| TC-8.5 | 拒绝后重新提现 | | | | |
| TC-9.1 | 佣金转余额 | | | | |
| TC-9.2 | 转余额余额不足 | | | | |
| TC-10.1 | 退款扣回 pending | | | | |
| TC-10.2 | 退款扣回 available | | | | |
| TC-10.3 | 已提现退款 negative carry | | | | |
| TC-11.1 | 手动增加佣金 | | | | |
| TC-11.2 | 手动扣减佣金 | | | | |
| TC-11.3 | 扣减超额 | | | | |
| TC-12.1 | 关系列表 | | | | |
| TC-12.2 | 修改邀请关系 | | | | |
| TC-12.3 | 关系变更历史 | | | | |
| TC-12.4 | 邀请树 | | | | |
| TC-12.5 | 管理总览 | | | | |
| TC-13.1 | 全局关闭菜单隐藏 | | | | |
| TC-13.2 | Per-User 开启 | | | | |
| TC-13.3 | 其他用户不可见 | | | | |
| TC-13.4 | Per-User 邀请码可用 | | | | |
| TC-13.5 | 未开启用户码不可用 | | | | |
| TC-13.6 | Per-User 提现 | | | | |
| TC-13.7 | 关闭 Per-User | | | | |
| TC-13.8 | 全局开启覆盖 | | | | |
| TC-14.1 | 非 CNY 货币 | | | | |
| TC-14.2 | 金额为 0 | | | | |
| TC-14.3 | 无关系用户充值 | | | | |
| TC-14.4 | 禁用的提现方式 | | | | |
| TC-14.5 | 调账超额 | | | | |
| TC-14.6 | 订单号冲突 | | | | |
| TC-14.7 | 充值后绑定限制 | | | | |
| TC-14.8 | 未认证访问 | | | | |
| TC-14.9 | 权限越权 | | | | |
