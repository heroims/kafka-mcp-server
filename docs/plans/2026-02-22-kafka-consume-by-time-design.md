# Kafka 时间范围消费功能设计

## 概述

新增 `consume_messages_by_time` MCP 工具，支持消费指定时间范围内的 Kafka 消息。

## 功能需求

- 新增独立的 MCP 工具 `consume_messages_by_time`
- 支持指定开始时间和结束时间（Unix 毫秒时间戳）
- 支持多主题消费
- 可选参数限制最大消息数

## MCP 工具设计

### 工具名称
`consume_messages_by_time`

### 参数
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| topics | string[] | 是 | 要消费的主题列表 |
| start_time | number | 是 | 开始时间，Unix 毫秒时间戳 |
| end_time | number | 是 | 结束时间，Unix 毫秒时间戳 |
| max_messages | number | 否 | 最大消息数，默认 10 |

### 返回值
JSON 数组，包含消息对象：
```json
[
  {
    "topic": "test-topic",
    "partition": 0,
    "offset": 123,
    "key": "key",
    "value": "message",
    "timestamp": 1705315800000
  }
]
```

## 接口设计

### KafkaClient 接口
```go
ConsumeMessagesByTime(ctx context.Context, topics []string, startTime, endTime int64, maxMessages int) ([]Message, error)
```

## 实现方案

### 1. ListOffsets 获取分区偏移量
- 对每个主题的每个分区，调用 `ListOffsets` API
- 分别获取 start_time 和 end_time 对应的 offset
- 使用 `kmsg.ListOffsetsRequest` 的 `Timestamp` 字段

### 2. 按偏移量消费
- 使用 `kgo.ConsumeOffsets()` 从起始 offset 消费到结束 offset
- 限制返回消息数不超过 maxMessages

### 3. 错误处理
- start_time >= end_time: 返回错误 "start_time must be less than end_time"
- 主题不存在: 返回错误
- 分区无数据: 跳过该分区

## 文件变更

1. `internal/kafka/interface.go` - 添加接口方法
2. `internal/kafka/client.go` - 实现方法
3. `internal/mcp/tools.go` - 添加 MCP 工具

## 测试计划

- 单元测试: 参数验证
- 集成测试: 使用 testcontainers 测试时间范围消费
