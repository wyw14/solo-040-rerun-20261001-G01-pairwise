package stowage

import (
	"context"

	"github.com/google/uuid"
	"github.com/wyw14/cry-96/internal/journal"
	"github.com/wyw14/cry-96/internal/model"
)

// PlanOccupancy reports how one active convoy plan uses the chamber.
type PlanOccupancy struct {
	PlanID     uuid.UUID `json:"plan_id"`
	Vessels    int       `json:"vessels"`
	OccupancyM float64   `json:"occupancy_m"`
	RemainingM float64   `json:"remaining_m"`
	Fits       bool      `json:"fits"`
}

// Status is the operator-facing stowage summary for one chamber.
type Status struct {
	Config    Config          `json:"config"`
	Default   bool            `json:"default"`
	Plans     []PlanOccupancy `json:"plans"`
	NextGroup *Group          `json:"next_group,omitempty"`
}

// Service applies stowage rules to plans and records configuration changes.
type Service struct {
	registry *Registry
	recorder *journal.Recorder
}

func NewService(registry *Registry, recorder *journal.Recorder) *Service {
	return &Service{registry: registry, recorder: recorder}
}

// Config returns the effective configuration for a chamber.
func (s *Service) Config(chamberID string) (Config, bool) {
	return s.registry.Get(chamberID)
}

// Configure validates, stores and journals a chamber configuration change.
func (s *Service) Configure(ctx context.Context, chamberID string, config Config) (Config, error) {
	stored, err := s.registry.Set(chamberID, config)
	if err != nil {
		return Config{}, err
	}
	if _, err := s.recorder.Record(ctx, chamberID, model.EventStowageConfigured, uuid.Nil, 0, stored); err != nil {
		return Config{}, err
	}
	return stored, nil
}

// Check verifies that the whole convoy fits one lockage; see package Check.
func (s *Service) Check(chamberID string, vessels []model.Vessel) error {
	config, _ := s.registry.Get(chamberID)
	return Check(chamberID, config, vessels)
}

// Groups packs the given vessels into lockage groups in queue order.
func (s *Service) Groups(chamberID string, vessels []model.Vessel) ([]Group, []VesselOccupancy) {
	config, _ := s.registry.Get(chamberID)
	return Layout(config, vessels)
}

// Status summarises configuration, active-plan occupancy and the next
// lockage group drawn from the chamber's queued (not yet released) vessels.
func (s *Service) Status(chamberID string, plans []model.ConvoyPlan, queued []model.Vessel) Status {
	config, isDefault := s.configWithSource(chamberID)
	status := Status{Config: config, Default: isDefault, Plans: make([]PlanOccupancy, 0, len(plans))}
	for _, plan := range plans {
		occupancy := Occupancy(config, plan.Vessels)
		status.Plans = append(status.Plans, PlanOccupancy{
			PlanID: plan.ID, Vessels: len(plan.Vessels), OccupancyM: occupancy,
			RemainingM: config.EffectiveLengthM - occupancy,
			Fits:       occupancy <= config.EffectiveLengthM+fitTolerance,
		})
	}
	groups, _ := Layout(config, queued)
	if len(groups) > 0 {
		next := groups[0]
		status.NextGroup = &next
	}
	return status
}

func (s *Service) configWithSource(chamberID string) (Config, bool) {
	config, configured := s.registry.Get(chamberID)
	return config, !configured
}
