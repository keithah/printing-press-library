// Copyright 2026 Keith Herrington and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"strings"
	"testing"
)

func TestMatchVehicles_ExactVsSubstring(t *testing.T) {
	rows := []vehicleRow{
		{DisplayName: "Car", VIN: "VCK5YJ3E1EA000001"},
		{DisplayName: "Carpenter", VIN: "VCK5YJ3E1EA000003"},
	}
	// Exact/preferred match must not widen to a substring-named vehicle.
	if got := matchVehicles(rows, "Car"); len(got) != 1 || got[0].DisplayName != "Car" {
		t.Fatalf("exact 'Car' = %+v", got)
	}
	if got := matchVehicles(rows, "car"); len(got) != 1 || got[0].DisplayName != "Car" {
		t.Fatalf("exact-preferred lowercase 'car' = %+v", got)
	}
	// VIN suffix matching.
	if got := matchVehicles(rows, "000001"); len(got) != 1 || got[0].VIN != "VCK5YJ3E1EA000001" {
		t.Fatalf("vin-suffix = %+v", got)
	}
	// Substring fallback only applies when no exact match.
	if got := matchVehicles(rows, "carpen"); len(got) != 1 || got[0].DisplayName != "Carpenter" {
		t.Fatalf("substring 'carpen' = %+v", got)
	}
	// No match.
	if got := matchVehicles(rows, "nonexistent"); len(got) != 0 {
		t.Fatalf("nonexistent = %+v", got)
	}
}

func TestMaskVIN(t *testing.T) {
	if got := maskVIN("VCK5YJ3E1EA000001"); !strings.HasSuffix(got, "0001") {
		t.Fatalf("maskVIN = %q", got)
	}
	if strings.Contains(maskVIN("VCK5YJ3E1EA000001"), "VCK") {
		t.Fatal("maskVIN leaked VIN prefix")
	}
	if got := maskVIN("ABCDE"); got != "***" {
		t.Fatalf("short vin mask = %q, want ***", got)
	}
}

func TestIsFullVIN(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"VCK5YJ3E1EA000001", true},
		{"7SAYGDED3TF664951", true},
		{"Car", false},
		{"5YJ3E1EA0", false},          // 10 chars
		{"VCK5YJ3E1EA0000012", false}, // 18 chars
		{"VCK-5YJ3E1EA000001", false}, // contains dash
		{"VCK5YJ3E1EA00000I", false},  // contains I
	}
	for _, c := range cases {
		if got := isFullVIN(c.in); got != c.want {
			t.Fatalf("isFullVIN(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestDisplayNameOrFallback(t *testing.T) {
	if got := displayNameOr(vehicleRow{DisplayName: "Car", VIN: "X"}); got != "Car" {
		t.Fatalf("display = %q", got)
	}
	if got := displayNameOr(vehicleRow{Plate: "AB12", VIN: "X"}); got != "AB12" {
		t.Fatalf("plate = %q", got)
	}
	if got := displayNameOr(vehicleRow{VIN: "VCK5YJ3E1EA000001"}); !strings.HasSuffix(got, "0001") {
		t.Fatalf("vin fallback = %q", got)
	}
}
