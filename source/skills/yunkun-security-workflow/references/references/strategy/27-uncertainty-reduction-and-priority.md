# Uncertainty Reduction And Priority

## 目标
有些测试动作的价值，不在于“像不像漏洞”，而在于它能快速改写系统模型。

## 推荐优先级

`Priority = Impact × Information Gain × Uncertainty Reduction × Confidence ÷ Cost`

## 什么时候不确定性收缩值高

- 一个结果会改变整类接口的权限判断
- 一个结果会决定当前架构是否存在新 trust boundary
- 一个结果能排除多个高分假设
- 一个结果能确认系统核心授权模型

## 例子

### /admin/export
- 若普通用户可访问：
  - 不只是单接口问题
  - 还会改写导出链、后台权限模型、同类接口优先级
