# Kafka 时间范围消费功能实现计划

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 新增 consume_messages_by_time MCP 工具，支持消费指定时间范围内的 Kafka 消息

**Architecture:** 使用 franz-go 的 ListOffsets API 获取时间对应的 offset，然后用 ConsumeOffsets 消费指定范围的消息

**Tech Stack:** Go, franz-go, MCP

---

## Task 1: 添加 KafkaClient 接口方法

**Files:**
- Modify: `internal/kafka/interface.go:1-48`

**Step 1: 添加接口方法**

在 KafkaClient 接口中添加新方法：

```go
// ConsumeMessagesByTime consumes messages from topics within a time range.
ConsumeMessagesByTime(ctx context.Context, topics []string, startTime, endTime int64, maxMessages int) ([]Message, error)
```

**Step 2: 验证接口实现**

确保 Client 实现了接口（文件底部已有 `var _ KafkaClient = (*Client)(nil)`）

**Step 3: Commit**

```bash
git add internal/kafka/interface.go
git commit -m "feat: add ConsumeMessagesByTime to KafkaClient interface"
```

---

## Task 2: 实现 ConsumeMessagesByTime 方法

**Files:**
- Modify: `internal/kafka/client.go:1-1005`

**Step 1: 添加方法实现**

在 client.go 文件末尾添加方法：

```go
// ConsumeMessagesByTime consumes messages from topics within a specified time range.
// It uses ListOffsets to find the offsets corresponding to startTime and endTime,
// then consumes messages within that range.
func (c *Client) ConsumeMessagesByTime(ctx context.Context, topics []string, startTime, endTime int64, maxMessages int) ([]Message, error) {
	if startTime >= endTime {
		return nil, fmt.Errorf("start_time must be less than end_time")
	}

	slog.InfoContext(ctx, "Consuming messages by time range", "topics", topics, "startTime", startTime, "endTime", endTime, "maxMessages", maxMessages)

	// Get metadata for topics to find partitions
	topicPartitions, err := c.getTopicPartitions(ctx, topics)
	if err != nil {
		return nil, fmt.Errorf("failed to get topic partitions: %w", err)
	}

	// Get start and end offsets for each partition
	offsets, err := c.getOffsetsForTimeRange(ctx, topicPartitions, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("failed to get offsets for time range: %w", err)
	}

	// Consume messages from start to end offset
	messages, err := c.consumeByOffsets(ctx, offsets, maxMessages)
	if err != nil {
		return nil, fmt.Errorf("failed to consume messages: %w", err)
	}

	slog.InfoContext(ctx, "Successfully consumed messages by time range", "count", len(messages))
	return messages, nil
}

// getTopicPartitions returns partition info for given topics
func (c *Client) getTopicPartitions(ctx context.Context, topics []string) (map[string][]int32, error) {
	req := kmsg.NewMetadataRequest()
	for _, topic := range topics {
		topicReq := kmsg.NewMetadataRequestTopic()
		topicReq.Topic = kmsg.StringPtr(topic)
		req.Topics = append(req.Topics, topicReq)
	}

	shardedResp := c.kgoClient.RequestSharded(ctx, &req)
	topicPartitions := make(map[string][]int32)

	for _, shard := range shardedResp {
		if shard.Err != nil {
			continue
		}
		resp, ok := shard.Resp.(*kmsg.MetadataResponse)
		if !ok {
			continue
		}
		for _, topic := range resp.Topics {
			if topic.Topic == nil || topic.ErrorCode != 0 {
				continue
			}
			for _, p := range topic.Partitions {
				topicPartitions[*topic.Topic] = append(topicPartitions[*topic.Topic], p.Partition)
			}
		}
	}

	return topicPartitions, nil
}

// getOffsetsForTimeRange gets start and end offsets for each topic-partition
func (c *Client) getOffsetsForTimeRange(ctx context.Context, topicPartitions map[string][]int32, startTime, endTime int64) (map[string]map[int32]OffsetRange, error) {
	offsets := make(map[string]map[int32]OffsetRange)

	// Get start offsets
	startOffsets, err := c.getOffsetsByTimestamp(ctx, topicPartitions, startTime)
	if err != nil {
		return nil, err
	}

	// Get end offsets
	endOffsets, err := c.getOffsetsByTimestamp(ctx, topicPartitions, endTime)
	if err != nil {
		return nil, err
	}

	// Combine into OffsetRange
	for topic, partitions := range topicPartitions {
		offsets[topic] = make(map[int32]OffsetRange)
		for _, partition := range partitions {
			startOffset := startOffsets[topic][partition]
			endOffset := endOffsets[topic][partition]
			if startOffset >= 0 && endOffset >= startOffset {
				offsets[topic][partition] = OffsetRange{
					Start: startOffset,
					End:   endOffset,
				}
			}
		}
	}

	return offsets, nil
}

// OffsetRange represents start and end offsets for a partition
type OffsetRange struct {
	Start int64
	End   int64
}

// getOffsetsByTimestamp gets the offset for each partition at a given timestamp
func (c *Client) getOffsetsByTimestamp(ctx context.Context, topicPartitions map[string][]int32, timestamp int64) (map[string]map[int32]int64, error) {
	req := kmsg.NewListOffsetsRequest()
	req.ReplicaID = -1

	for topic, partitions := range topicPartitions {
		topicReq := kmsg.NewListOffsetsRequestTopic()
		topicReq.Topic = topic
		for _, partition := range partitions {
			partitionReq := kmsg.NewListOffsetsRequestTopicPartition()
			partitionReq.Partition = partition
			partitionReq.Timestamp = timestamp
			topicReq.Partitions = append(topicReq.Partitions, partitionReq)
		}
		req.Topics = append(req.Topics, topicReq)
	}

	shardedResp := c.kgoClient.RequestSharded(ctx, &req)
	result := make(map[string]map[int32]int64)

	for topic := range topicPartitions {
		result[topic] = make(map[int32]int64)
	}

	for _, shard := range shardedResp {
		if shard.Err != nil {
			continue
		}
		resp, ok := shard.Resp.(*kmsg.ListOffsetsResponse)
		if !ok {
			continue
		}
		for _, topic := range resp.Topics {
			for _, partition := range topic.Partitions {
				if partition.ErrorCode == 0 {
					result[topic.Topic][partition.Partition] = partition.Offset
				}
			}
		}
	}

	return result, nil
}

// consumeByOffsets consumes messages from specified offset ranges
func (c *Client) consumeByOffsets(ctx context.Context, offsets map[string]map[int32]OffsetRange, maxMessages int) ([]Message, error) {
	// Build fetch offsets request
	opts := []kgo.Opt{
		kgo.FetchOffsets(offsets),
		kgo.MaxPollRecords(int32(maxMessages)),
	}

	// Create a temporary client with the specific offsets
	fetchClient, err := kgo.NewClient(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create fetch client: %w", err)
	}
	defer fetchClient.Close()

	fetches := fetchClient.PollFetches(ctx)
	if fetches.IsClientClosed() {
		return nil, fmt.Errorf("client is closed")
	}

	errs := fetches.Errors()
	if len(errs) > 0 {
		return nil, fmt.Errorf("fetch error: %w", errs[0].Err)
	}

	messages := make([]Message, 0, maxMessages)
	iter := fetches.RecordIter()
	count := 0

	for !iter.Done() && count < maxMessages {
		rec := iter.Next()
		messages = append(messages, Message{
			Topic:     rec.Topic,
			Partition: rec.Partition,
			Offset:    rec.Offset,
			Key:       string(rec.Key),
			Value:     string(rec.Value),
			Timestamp: rec.Timestamp.UnixMilli(),
		})
		count++
	}

	return messages, nil
}
```

**Step 2: 测试编译**

```bash
go build ./...
```

**Step 3: Commit**

```bash
git add internal/kafka/client.go
git commit -m "feat: implement ConsumeMessagesByTime in Kafka client"
```

---

## Task 3: 添加 MCP 工具

**Files:**
- Modify: `internal/mcp/tools.go:1-374`

**Step 1: 添加 consume_messages_by_time 工具**

在 tools.go 文件末尾添加：

```go
// --- consume_messages_by_time tool definition and handler ---
consumeByTimeTool := mcp.NewTool("consume_messages_by_time",
	mcp.WithDescription("Consumes messages from Kafka topics within a specified time range. Use this tool to retrieve messages that were produced during a specific time period. Requires specifying start and end timestamps in Unix milliseconds."),
	mcp.WithArray("topics", mcp.Required(), mcp.Description("Array of Kafka topic names to consume messages from."), mcp.Items(map[string]any{"type": "string"})),
	mcp.WithNumber("start_time", mcp.Required(), mcp.Description("Start time in Unix milliseconds (e.g., 1705315800000). Messages after this time will be consumed.")),
	mcp.WithNumber("end_time", mcp.Required(), mcp.Description("End time in Unix milliseconds. Messages before this time will be consumed.")),
	mcp.WithNumber("max_messages", mcp.Description("Maximum number of messages to consume (default: 10).")),
)

s.AddTool(consumeByTimeTool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	topicsArg := req.GetStringSlice("topics", []string{})
	startTime := int64(req.GetFloat("start_time", 0))
	endTime := int64(req.GetFloat("end_time", 0))
	maxMessages := int(req.GetFloat("max_messages", 10))

	if len(topicsArg) == 0 {
		return mcp.NewToolResultError("No valid topics provided."), nil
	}
	if startTime <= 0 || endTime <= 0 {
		return mcp.NewToolResultError("Invalid start_time or end_time: must be positive Unix timestamps"), nil
	}
	if startTime >= endTime {
		return mcp.NewToolResultError("start_time must be less than end_time"), nil
	}

	slog.InfoContext(ctx, "Executing consume_messages_by_time tool", "topics", topicsArg, "startTime", startTime, "endTime", endTime, "maxMessages", maxMessages)

	messages, err := kafkaClient.ConsumeMessagesByTime(ctx, topicsArg, startTime, endTime, maxMessages)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to consume messages by time", "error", err)
		return mcp.NewToolResultError(fmt.Sprintf("Failed to consume messages: %v", err)), nil
	}

	slog.InfoContext(ctx, "Successfully consumed messages by time", "count", len(messages))

	jsonData, marshalErr := json.Marshal(messages)
	if marshalErr != nil {
		slog.ErrorContext(ctx, "Failed to marshal messages to JSON", "error", marshalErr)
		return mcp.NewToolResultError("Internal server error: failed to marshal results"), nil
	}

	return mcp.NewToolResultText(string(jsonData)), nil
})
```

**Step 2: 测试编译**

```bash
go build ./...
```

**Step 3: Commit**

```bash
git add internal/mcp/tools.go
git commit -m "feat: add consume_messages_by_time MCP tool"
```

---

## Task 4: 添加集成测试

**Files:**
- Modify: `internal/kafka/client_test.go:1-198`

**Step 1: 添加测试用例**

```go
func TestConsumeMessagesByTime(t *testing.T) {
	if testKafkaContainer == nil {
		t.Skip("Skipping test: Kafka container not available")
	}
	require.NotEmpty(t, testKafkaBrokers, "Kafka brokers should be set by TestMain")

	cfg := getTestConfig()
	client, err := NewClient(cfg)
	require.NoError(t, err)
	require.NotNil(t, client)
	defer client.Close()

	ctx := context.Background()
	topic := "test-time-range-" + time.Now().Format("20060102150405")

	// Produce messages with timestamps
	now := time.Now().UnixMilli()
	for i := 0; i < 5; i++ {
		err = client.ProduceMessage(ctx, topic, nil, []byte(fmt.Sprintf("message-%d", i)))
		require.NoError(t, err)
		time.Sleep(100 * time.Millisecond)
	}

	// Wait for messages to be available
	time.Sleep(2 * time.Second)

	// Consume by time range
	startTime := now - 1000 // 1 second ago
	endTime := time.Now().UnixMilli()

	messages, err := client.ConsumeMessagesByTime(ctx, []string{topic}, startTime, endTime, 10)
	require.NoError(t, err)
	require.NotEmpty(t, messages, "Should consume messages within time range")

	// Verify messages are within range
	for _, msg := range messages {
		assert.True(t, msg.Timestamp >= startTime, "Message timestamp should be >= start_time")
		assert.True(t, msg.Timestamp <= endTime, "Message timestamp should be <= end_time")
	}
}
```

**Step 2: 运行测试**

```bash
go test -v -run TestConsumeMessagesByTime ./internal/kafka
```

**Step 3: Commit**

```bash
git add internal/kafka/client_test.go
git commit -m "test: add ConsumeMessagesByTime integration test"
```

---

## Task 5: 运行 Lint

**Step 1: 运行 linter**

```bash
make lint
```

**Step 2: 如果有错误，修复后重新提交**

---

## 总结

完成以上 5 个任务后，功能实现完毕。变更文件：
1. `internal/kafka/interface.go` - 接口定义
2. `internal/kafka/client.go` - 实现
3. `internal/mcp/tools.go` - MCP 工具
4. `internal/kafka/client_test.go` - 测试
