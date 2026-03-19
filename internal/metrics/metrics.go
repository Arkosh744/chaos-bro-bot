package metrics

import (
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"
)

type Store struct {
	messagesTotal     atomic.Int64
	claudeCallsTotal  atomic.Int64
	claudeErrorsTotal atomic.Int64
	claudeLatencySum  atomic.Int64
	claudeCallCount   atomic.Int64
	activeUsers       sync.Map // userID -> last seen unix timestamp
}

var Global = &Store{}

func IncrementMessages()    { Global.messagesTotal.Add(1) }
func IncrementClaudeCalls() { Global.claudeCallsTotal.Add(1) }
func IncrementClaudeErrors() { Global.claudeErrorsTotal.Add(1) }

func RecordClaudeLatency(ms int64) {
	Global.claudeLatencySum.Add(ms)
	Global.claudeCallCount.Add(1)
}

func TrackActiveUser(userID int64) {
	Global.activeUsers.Store(userID, time.Now().Unix())
}

func WriteTo(w io.Writer) {
	var activeCount int
	now := time.Now().Unix()
	Global.activeUsers.Range(func(k, v any) bool {
		if now-v.(int64) < 3600 {
			activeCount++
		}
		return true
	})

	avgLatency := int64(0)
	if count := Global.claudeCallCount.Load(); count > 0 {
		avgLatency = Global.claudeLatencySum.Load() / count
	}

	fmt.Fprintf(w, "# HELP messages_total Total messages processed\n")
	fmt.Fprintf(w, "# TYPE messages_total counter\n")
	fmt.Fprintf(w, "messages_total %d\n", Global.messagesTotal.Load())
	fmt.Fprintf(w, "# HELP claude_calls_total Total Claude API calls\n")
	fmt.Fprintf(w, "# TYPE claude_calls_total counter\n")
	fmt.Fprintf(w, "claude_calls_total %d\n", Global.claudeCallsTotal.Load())
	fmt.Fprintf(w, "# HELP claude_errors_total Total Claude API errors\n")
	fmt.Fprintf(w, "# TYPE claude_errors_total counter\n")
	fmt.Fprintf(w, "claude_errors_total %d\n", Global.claudeErrorsTotal.Load())
	fmt.Fprintf(w, "# HELP claude_avg_latency_ms Average Claude call latency in ms\n")
	fmt.Fprintf(w, "# TYPE claude_avg_latency_ms gauge\n")
	fmt.Fprintf(w, "claude_avg_latency_ms %d\n", avgLatency)
	fmt.Fprintf(w, "# HELP active_users_1h Users active in the last hour\n")
	fmt.Fprintf(w, "# TYPE active_users_1h gauge\n")
	fmt.Fprintf(w, "active_users_1h %d\n", activeCount)
}
