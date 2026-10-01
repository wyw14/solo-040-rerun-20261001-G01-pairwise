package stowage

import (
	"strings"
	"testing"
	"time"

	"github.com/wyw14/cry-96/internal/model"
)

func testConfig() Config {
	return Config{EffectiveLengthM: 100, SafetySpacingM: 10}
}

func vessel(t *testing.T, name string, length float64) model.Vessel {
	t.Helper()
	v, err := model.NewVessel(name, "CS-"+name, length)
	if err != nil {
		t.Fatalf("vessel %s: %v", name, err)
	}
	return v
}

func TestLayoutPacksSingleGroup(t *testing.T) {
	vessels := []model.Vessel{vessel(t, "A", 40), vessel(t, "B", 30)}
	groups, unplaceable := Layout(testConfig(), vessels)
	if len(unplaceable) != 0 {
		t.Fatalf("unexpected unplaceable: %v", unplaceable)
	}
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	group := groups[0]
	if group.OccupancyM != 80 || group.RemainingM != 20 {
		t.Fatalf("occupancy=%v remaining=%v, want 80/20", group.OccupancyM, group.RemainingM)
	}
	if group.Vessels[0].SpacingM != 10 || group.Vessels[0].OccupancyM != 50 {
		t.Fatalf("first vessel should carry the spacing: %+v", group.Vessels[0])
	}
	if group.Vessels[1].SpacingM != 0 || group.Vessels[1].OccupancyM != 30 {
		t.Fatalf("last vessel carries no spacing: %+v", group.Vessels[1])
	}
}

func TestLayoutSplitsInQueueOrder(t *testing.T) {
	vessels := []model.Vessel{vessel(t, "A", 40), vessel(t, "B", 50), vessel(t, "C", 30)}
	groups, unplaceable := Layout(testConfig(), vessels)
	if len(unplaceable) != 0 {
		t.Fatalf("unexpected unplaceable: %v", unplaceable)
	}
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
	if got := groups[0].Vessels[1].Vessel.Name; got != "B" {
		t.Fatalf("queue order broken, group 1 second vessel = %s", got)
	}
	if groups[0].OccupancyM != 100 || groups[0].RemainingM != 0 {
		t.Fatalf("group 1 occupancy=%v remaining=%v, want 100/0", groups[0].OccupancyM, groups[0].RemainingM)
	}
	if groups[1].Vessels[0].Vessel.Name != "C" || groups[1].OccupancyM != 30 {
		t.Fatalf("group 2 should hold C alone: %+v", groups[1])
	}
}

func TestLayoutExactBoundaryFits(t *testing.T) {
	vessels := []model.Vessel{vessel(t, "A", 45), vessel(t, "B", 45)}
	groups, _ := Layout(testConfig(), vessels)
	if len(groups) != 1 {
		t.Fatalf("exact 100m convoy must fit one lockage, got %d groups", len(groups))
	}
}

func TestLayoutUnplaceableVessel(t *testing.T) {
	groups, unplaceable := Layout(testConfig(), []model.Vessel{vessel(t, "A", 150)})
	if len(groups) != 0 {
		t.Fatalf("oversized vessel must not form a group: %+v", groups)
	}
	if len(unplaceable) != 1 || unplaceable[0].OccupancyM != 150 {
		t.Fatalf("unplaceable should list the vessel at its length: %+v", unplaceable)
	}
}

func TestCheckAcceptsFittingConvoy(t *testing.T) {
	vessels := []model.Vessel{vessel(t, "A", 40), vessel(t, "B", 30)}
	if err := Check("chamber-1", testConfig(), vessels); err != nil {
		t.Fatalf("fitting convoy rejected: %v", err)
	}
	if err := Check("chamber-1", testConfig(), nil); err != nil {
		t.Fatalf("empty convoy rejected: %v", err)
	}
}

func TestCheckOverflowNamesVesselsAndOccupancy(t *testing.T) {
	vessels := []model.Vessel{vessel(t, "A", 40), vessel(t, "B", 50), vessel(t, "C", 30)}
	err := Check("chamber-1", testConfig(), vessels)
	overflow, ok := err.(*Overflow)
	if !ok {
		t.Fatalf("expected *Overflow, got %T (%v)", err, err)
	}
	if overflow.RequiredM != 140 || overflow.ExcessM != 40 {
		t.Fatalf("required=%v excess=%v, want 140/40", overflow.RequiredM, overflow.ExcessM)
	}
	if len(overflow.Fits) != 2 || overflow.Fits[0].Vessel.Name != "A" || overflow.Fits[1].Vessel.Name != "B" {
		t.Fatalf("fits should be A+B in queue order: %+v", overflow.Fits)
	}
	if len(overflow.Excluded) != 1 || overflow.Excluded[0].Vessel.Name != "C" || overflow.Excluded[0].OccupancyM != 30 {
		t.Fatalf("excluded should name C at 30m: %+v", overflow.Excluded)
	}
	if !strings.Contains(err.Error(), "C (30.0m)") {
		t.Fatalf("error message should name the excluded vessel: %v", err)
	}
}

func TestCheckUnplaceableVesselOverflows(t *testing.T) {
	err := Check("chamber-1", testConfig(), []model.Vessel{vessel(t, "A", 150)})
	overflow, ok := err.(*Overflow)
	if !ok {
		t.Fatalf("expected *Overflow, got %T (%v)", err, err)
	}
	if len(overflow.Fits) != 0 || len(overflow.Excluded) != 1 {
		t.Fatalf("oversized vessel: fits=%+v excluded=%+v", overflow.Fits, overflow.Excluded)
	}
}

func TestRegistryDefaultsAndValidation(t *testing.T) {
	registry, err := NewRegistry(testConfig(), time.Now)
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	config, configured := registry.Get("chamber-9")
	if configured || config.EffectiveLengthM != 100 {
		t.Fatalf("unknown chamber should fall back to defaults: %+v", config)
	}
	if _, err := registry.Set("chamber-9", Config{EffectiveLengthM: 0, SafetySpacingM: 1}); err == nil {
		t.Fatal("zero effective length must be rejected")
	}
	if _, err := registry.Set("chamber-9", Config{EffectiveLengthM: 100, SafetySpacingM: 100}); err == nil {
		t.Fatal("spacing >= effective length must be rejected")
	}
	if _, err := registry.Set("chamber-9", Config{EffectiveLengthM: 100, SafetySpacingM: -1}); err == nil {
		t.Fatal("negative spacing must be rejected")
	}
	stored, err := registry.Set("chamber-9", Config{EffectiveLengthM: 200, SafetySpacingM: 8})
	if err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
	if stored.UpdatedAt.IsZero() {
		t.Fatal("stored config should be timestamped")
	}
	config, configured = registry.Get("chamber-9")
	if !configured || config.EffectiveLengthM != 200 || config.SafetySpacingM != 8 {
		t.Fatalf("configured value not returned: %+v", config)
	}
}
