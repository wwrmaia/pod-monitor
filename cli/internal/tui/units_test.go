package tui

import (
	"math"
	"testing"
)

const floatEps = 1e-9

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < floatEps
}

func TestParseCPUMilli(t *testing.T) {
	cases := []struct {
		in   string
		want float64
		ok   bool
	}{
		{"0", 0, true},
		{"250m", 250, true},
		{"1", 1000, true},
		{"1500m", 1500, true},
		{"0.5", 500, true},
		{"146268n", 0.146268, true}, // formato real observado no cpu_usage do metrics-server
		{"500u", 0.5, true},
		{"", 0, false},
		{"garbage", 0, false},
	}
	for _, c := range cases {
		got, ok := parseCPUMilli(c.in)
		if ok != c.ok || (ok && !almostEqual(got, c.want)) {
			t.Errorf("parseCPUMilli(%q) = (%v,%v), want (%v,%v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestParseMemMiB(t *testing.T) {
	cases := []struct {
		in   string
		want float64
		ok   bool
	}{
		{"128Mi", 128, true},
		{"1Gi", 1024, true},
		{"512Ki", 0.5, true},
		{"", 0, false},
		{"garbage", 0, false},
	}
	for _, c := range cases {
		got, ok := parseMemMiB(c.in)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("parseMemMiB(%q) = (%v,%v), want (%v,%v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestPctOf(t *testing.T) {
	pct, ok := pctOf("450m", "500m", parseCPUMilli)
	if !ok || pct != 90 {
		t.Errorf("pctOf(450m,500m) = (%v,%v), want (90,true)", pct, ok)
	}
	if _, ok := pctOf("", "500m", parseCPUMilli); ok {
		t.Errorf("pctOf with empty usage should be ok=false")
	}
	if _, ok := pctOf("100m", "", parseCPUMilli); ok {
		t.Errorf("pctOf with empty limit should be ok=false")
	}
}

func TestSevForPct(t *testing.T) {
	if got := sevForPct(50, true); got != "" {
		t.Errorf("sevForPct(50) = %q, want \"\"", got)
	}
	if got := sevForPct(87, true); got != "warning" {
		t.Errorf("sevForPct(87) = %q, want warning", got)
	}
	if got := sevForPct(95, true); got != "critical" {
		t.Errorf("sevForPct(95) = %q, want critical", got)
	}
	if got := sevForPct(99, false); got != "" {
		t.Errorf("sevForPct(99, ok=false) = %q, want \"\"", got)
	}
}
