package stowage

import (
	"context"
	"testing"
	"time"

	"github.com/wyw14/cry-96/internal/journal"
	"github.com/wyw14/cry-96/internal/model"
)

func TestConfigureRecordsEvent(t *testing.T) {
	now := func() time.Time { return time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC) }
	store := journal.NewMemoryStore()
	recorder := journal.NewRecorder(store, now)
	registry, err := NewRegistry(Config{EffectiveLengthM: 100, SafetySpacingM: 10}, now)
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	service := NewService(registry, recorder)
	stored, err := service.Configure(context.Background(), "chamber-1", Config{EffectiveLengthM: 200, SafetySpacingM: 8})
	if err != nil {
		t.Fatalf("configure: %v", err)
	}
	if stored.EffectiveLengthM != 200 || stored.UpdatedAt.IsZero() {
		t.Fatalf("stored config incomplete: %+v", stored)
	}
	events, err := store.Events(context.Background(), "chamber-1", 0)
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	if len(events) != 1 || events[0].Type != model.EventStowageConfigured {
		t.Fatalf("expected one stowage.configured event, got %+v", events)
	}
	var payload Config
	if err := events[0].Decode(&payload); err != nil || payload.EffectiveLengthM != 200 {
		t.Fatalf("event payload should carry the config: %v %+v", err, payload)
	}
	if _, err := service.Configure(context.Background(), "chamber-1", Config{EffectiveLengthM: -1}); err == nil {
		t.Fatal("invalid config must be rejected before any event is recorded")
	}
	events, _ = store.Events(context.Background(), "chamber-1", 0)
	if len(events) != 1 {
		t.Fatalf("rejected config must not record events, got %d", len(events))
	}
}

func TestStatusSummarisesPlansAndNextGroup(t *testing.T) {
	now := func() time.Time { return time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC) }
	registry, err := NewRegistry(Config{EffectiveLengthM: 100, SafetySpacingM: 10}, now)
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	service := NewService(registry, journal.NewRecorder(journal.NewMemoryStore(), now))
	plan, err := model.NewConvoyPlan("chamber-1", model.DirectionUpstream, []model.Vessel{
		vessel(t, "A", 40), vessel(t, "B", 30),
	}, now())
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	queued := []model.Vessel{vessel(t, "C", 60), vessel(t, "D", 60)}
	status := service.Status("chamber-1", []model.ConvoyPlan{plan}, queued)
	if status.Config.EffectiveLengthM != 100 || !status.Default {
		t.Fatalf("status should expose the default config: %+v", status.Config)
	}
	if len(status.Plans) != 1 || status.Plans[0].OccupancyM != 80 || status.Plans[0].RemainingM != 20 || !status.Plans[0].Fits {
		t.Fatalf("plan occupancy wrong: %+v", status.Plans)
	}
	if status.NextGroup == nil || len(status.NextGroup.Vessels) != 1 || status.NextGroup.Vessels[0].Vessel.Name != "C" {
		t.Fatalf("next group should hold C alone (60+10+60 > 100): %+v", status.NextGroup)
	}
	empty := service.Status("chamber-1", nil, nil)
	if empty.NextGroup != nil || len(empty.Plans) != 0 {
		t.Fatalf("empty chamber should have no group and no plans: %+v", empty)
	}
}
