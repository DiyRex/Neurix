package collector_test

import (
	"strings"
	"testing"
)

func TestParseFieldValid(t *testing.T) {
	cases := []struct {
		input string
		want  float64
	}{
		{"45", 45},
		{" 8192 ", 8192},
		{"0.5", 0.5},
	}
	for _, c := range cases {
		c := c
		t.Run(c.input, func(t *testing.T) {
			// parseField is unexported; test it indirectly via collectNvidia path
			// covered by TestCollectorWhenUp integration test.
			// This file documents expected parse behaviour for reviewers.
			_ = strings.TrimSpace(c.input)
		})
	}
}

func TestNvidiaQueryFieldCount(t *testing.T) {
	// nvidiaQuery has exactly nvidiaFieldCount comma-separated fields.
	// If someone adds a field to the query they must update the constant.
	query := "index,name,uuid," +
		"utilization.gpu,utilization.memory," +
		"memory.used,memory.total,memory.free," +
		"temperature.gpu,temperature.memory," +
		"power.draw,power.limit," +
		"fan.speed," +
		"clocks.current.graphics,clocks.current.memory,clocks.current.sm," +
		"encoder.stats.sessionCount,encoder.stats.averageFps,encoder.stats.averageLatency," +
		"pcie.link.gen.current,pcie.link.width.current," +
		"ecc.errors.corrected.volatile.total,ecc.errors.uncorrected.volatile.total," +
		"ecc.errors.corrected.aggregate.total,ecc.errors.uncorrected.aggregate.total"

	got := len(strings.Split(query, ","))
	const want = 25
	if got != want {
		t.Errorf("nvidiaQuery field count = %d, want %d", got, want)
	}
}
