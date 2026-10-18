# Lane: Smart Contract And DeFi Analysis

## 适用信号

- Solidity、Vyper、Move、Foundry、Hardhat、Anchor、合约 ABI、部署地址或链上交易数据。

## 最小模型

1. 标出资产、角色、授权入口、价格来源、外部调用、升级路径、清算与跨链边界。
2. 从一条具体状态转换开始：谁输入、合约如何校验、哪些余额/份额/权限改变、如何回滚或复现。
3. 先在本地测试、fork 或现有测试网环境验证；不要把链上历史案例当作当前合约漏洞。

## 专项分支

- access control / initializer / upgrade
- accounting / share conversion / precision
- oracle / price freshness / liquidation
- external call / callback / reentrancy
- signature / permit / replay / domain separation
- bridge / cross-chain message validation

## 证据门槛

- finding 必须有受影响函数、前置状态、最小交易序列、断言和预期/实际状态差异。
- 未能在本地或受控 fork 复现时，只记录为 lead。
- 只在出现合约信号时加载本 lane；普通 Web/API 任务不加载。
