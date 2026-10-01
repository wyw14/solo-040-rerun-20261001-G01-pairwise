package schedule

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/wyw14/cry-96/internal/admission"
	"github.com/wyw14/cry-96/internal/gate"
	"github.com/wyw14/cry-96/internal/journal"
	"github.com/wyw14/cry-96/internal/model"
	"github.com/wyw14/cry-96/internal/signal"
	"github.com/wyw14/cry-96/internal/stowage"
)

func newTestPlanner(t *testing.T, config stowage.Config) (*Planner, *admission.Queue, *journal.MemoryStore) {
	t.Helper()
	now := func() time.Time { return time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC) }
	store := journal.NewMemoryStore()
	recorder := journal.NewRecorder(store, now)
	queue := admission.NewQueue()
	controller := admission.NewController(queue, gate.NewState("chamber-1"), signal.NewStore(), now)
	admissionService := admission.NewService(controller, recorder, now)
	registry, err := stowage.NewRegistry(config, now)
	if err != nil {
		t.Fatalf("stowage registry: %v", err)
	}
	planner := NewPlanner(admissionService, stowage.NewService(registry, recorder), recorder, now)
	return planner, queue, store
}

func testVessel(t *testing.T, name string, length float64) model.Vessel {
	t.Helper()
	vessel, err := model.NewVessel(name, "CS-"+name, length)
	if err != nil {
		t.Fatalf("vessel %s: %v", name, err)
	}
	return vessel
}

func TestCreateFittingPlanKeepsExistingFlow(t *testing.T) {
	planner, queue, store := newTestPlanner(t, stowage.Config{EffectiveLengthM: 100, SafetySpacingM: 10})
	vessels := []model.Vessel{testVessel(t, "A", 40), testVessel(t, "B", 30)}
	plan, err := planner.Create(context.Background(), "chamber-1", model.DirectionUpstream, vessels)
	if err != nil {
		t.Fatalf("fitting convoy rejected: %v", err)
	}
	if len(plan.Vessels) != 2 {
		t.Fatalf("plan vessels = %d, want 2", len(plan.Vessels))
	}
	if active := planner.Active("chamber-1"); len(active) != 1 {
		t.Fatalf("active plans = %d, want 1", len(active))
	}
	if queued := queue.List("chamber-1"); len(queued) != 2 {
		t.Fatalf("queued vessels = %d, want 2", len(queued))
	}
	events, err := store.Events(context.Background(), "chamber-1", 0)
	if err != nil || len(events) == 0 {
		t.Fatalf("plan creation events missing: %v %d", err, len(events))
	}
	for _, event := range events {
		if event.Type != model.EventPlanCreated {
			t.Fatalf("unexpected event type %s", event.Type)
		}
	}
}

func TestCreateOverflowCreatesNoPartialPlan(t *testing.T) {
	planner, queue, store := newTestPlanner(t, stowage.Config{EffectiveLengthM: 100, SafetySpacingM: 10})
	vessels := []model.Vessel{testVessel(t, "A", 40), testVessel(t, "B", 50), testVessel(t, "C", 30)}
	_, err := planner.Create(context.Background(), "chamber-1", model.DirectionUpstream, vessels)
	var overflow *stowage.Overflow
	if !errors.As(err, &overflow) {
		t.Fatalf("expected *stowage.Overflow, got %T (%v)", err, err)
	}
	if len(overflow.Excluded) != 1 || overflow.Excluded[0].Vessel.Name != "C" {
		t.Fatalf("overflow should name vessel C: %+v", overflow.Excluded)
	}
	if active := planner.Active("chamber-1"); len(active) != 0 {
		t.Fatalf("partial plan leaked into active plans: %+v", active)
	}
	if queued := queue.List("chamber-1"); len(queued) != 0 {
		t.Fatalf("partial plan leaked into admission queue: %+v", queued)
	}
	events, err := store.Events(context.Background(), "chamber-1", 0)
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("rejected plan must not record events, got %d", len(events))
	}
}

func TestCreateUnplaceableVesselRejected(t *testing.T) {
	planner, queue, _ := newTestPlanner(t, stowage.Config{EffectiveLengthM: 100, SafetySpacingM: 10})
	_, err := planner.Create(context.Background(), "chamber-1", model.DirectionUpstream, []model.Vessel{testVessel(t, "A", 150)})
	var overflow *stowage.Overflow
	if !errors.As(err, &overflow) {
		t.Fatalf("expected *stowage.Overflow, got %T (%v)", err, err)
	}
	if len(overflow.Fits) != 0 || len(overflow.Excluded) != 1 {
		t.Fatalf("oversized vessel: fits=%+v excluded=%+v", overflow.Fits, overflow.Excluded)
	}
	if queued := queue.List("chamber-1"); len(queued) != 0 {
		t.Fatalf("queue must stay empty, got %+v", queued)
	}
}
