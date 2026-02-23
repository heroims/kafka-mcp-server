package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/tuannvm/kafka-mcp-server/internal/config"
	"github.com/tuannvm/kafka-mcp-server/internal/kafka"
)

func main() {
	brokers := []string{
		"alikafka-pre-public-intl-sg-x1e4eic8i02-1-vpc.alikafka.aliyuncs.com:9092",
		"alikafka-pre-public-intl-sg-x1e4eic8i02-2-vpc.alikafka.aliyuncs.com:9092",
		"alikafka-pre-public-intl-sg-x1e4eic8i02-3-vpc.alikafka.aliyuncs.com:9092",
	}

	cfg := config.Config{
		KafkaBrokers:  brokers,
		KafkaClientID: "test-client",
	}

	client, err := kafka.NewClient(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	ctx := context.Background()

	// Feb 22, 2026 12:24:58 - 12:25:58 (actual data in topic)
	startTime := int64(1771734298000)
	endTime := int64(1771734308000)
	maxLimit := 5000
	fmt.Printf("Time range: %s - %s\n\n",
		time.UnixMilli(startTime).Format(time.DateTime),
		time.UnixMilli(endTime).Format(time.DateTime))

	// Test 1: reset=true
	fmt.Println("=== Test 1: reset=true (first call) ===")
	messages, err := client.ConsumeMessagesByTime(ctx, []string{"web3_token_events"}, startTime, endTime, maxLimit, true)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	} else {
		fmt.Printf("Got %d messages\n", len(messages))
		for i, m := range messages {
			ts := time.UnixMilli(m.Timestamp)
			fmt.Printf("[%d] p%d offset=%d ts=%d (%s)\n", i, m.Partition, m.Offset, m.Timestamp, ts.Format(time.DateTime))
		}
	}

	// Test 2: reset=false (resume - should get different messages or empty if committed)
	fmt.Println("\n=== Test 2: reset=false (resume) ===")
	messages2, err := client.ConsumeMessagesByTime(ctx, []string{"web3_token_events"}, startTime, endTime, maxLimit, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	} else {
		fmt.Printf("Got %d messages\n", len(messages2))
		for i, m := range messages2 {
			ts := time.UnixMilli(m.Timestamp)
			fmt.Printf("[%d] p%d offset=%d ts=%d (%s)\n", i, m.Partition, m.Offset, m.Timestamp, ts.Format(time.DateTime))
		}
	}

	// Test 3: Larger time range
	fmt.Println("\n=== Test 3: Larger time range (16:50:38 - 16:55:00) ===")
	messages3, err := client.ConsumeMessagesByTime(ctx, []string{"web3_token_events"}, startTime, endTime, maxLimit, true)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	} else {
		fmt.Printf("Got %d messages\n", len(messages3))
		for i, m := range messages3 {
			ts := time.UnixMilli(m.Timestamp)
			fmt.Printf("[%d] p%d offset=%d ts=%d (%s)\n", i, m.Partition, m.Offset, m.Timestamp, ts.Format(time.DateTime))
		}
	}
}
