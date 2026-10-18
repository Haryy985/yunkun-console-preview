# 30 · SRC 目标画像与资产优先级

## 目标

把 `主体 / 根域名 / App / 小程序 / 规则页 / 已知资产` 转成可执行的首轮测试队列，而不是平均扫。

## 固定顺序

1. `subject map`：主体、品牌、学院/子公司、历史系统、账号体系、跳转关系
2. `scope map`：规则允许的域名、子系统、资产形态、禁止动作
3. `entry map`：登录、注册、找回、管理后台、导出、上传、支付、对象存储、Webhook、GraphQL、App/小程序
4. `asset ranking`：给每个入口打分并产出 `P0 / P1 / P2`

## 六维排序

- `business_criticality`
- `object_permission_density`
- `verifiability`
- `reportability`
- `exposure`
- `duplicate_noise_risk`

优先级公式：

`(business_criticality + object_permission_density + verifiability + reportability + exposure) - duplicate_noise_risk`

## 默认 P0 信号

- `login / register / reset / bind / sso / oauth`
- `userId / tenantId / roleId / orgId / fileId / orderId`
- `export / download / report / approval / audit`
- `upload / preview / convert / callback / webhook`
- `amount / coupon / balance / refund / payment`
- `bucket / sts / ak/sk / signed url`
- `swagger / openapi / graphql / source map / hidden route`

## 产出模板

```text
subject_profile:
scope_to_asset_map:
high_value_entries:
p0_queue:
p1_queue:
blocked_prereqs:
```
