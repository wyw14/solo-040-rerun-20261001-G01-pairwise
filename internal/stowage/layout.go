package stowage

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/wyw14/cry-96/internal/model"
)

// fitTolerance absorbs floating-point dust when comparing metre totals.
const fitTolerance = 1e-9

// VesselOccupancy is one vessel's footprint inside a chamber group.
type VesselOccupancy struct {
	Vessel     model.Vessel `json:"vessel"`
	SpacingM   float64      `json:"spacing_m"`
	OccupancyM float64      `json:"occupancy_m"`
}

// Group is the set of vessels admitted together in one lockage.
type Group struct {
	Sequence   int               `json:"sequence"`
	Vessels    []VesselOccupancy `json:"vessels"`
	OccupancyM float64           `json:"occupancy_m"`
	RemainingM float64           `json:"remaining_m"`
}

// Layout walks vessels in queue order and packs them into lockage groups.
// A vessel is added to the open group while its length plus the spacing
// towards its predecessor stays within the effective length; otherwise the
// group is closed and a new one starts. Vessels longer than the effective
// length can never enter and are returned as unplaceable.
func Layout(config Config, vessels []model.Vessel) (groups []Group, unplaceable []VesselOccupancy) {
	var current []model.Vessel
	used := 0.0
	closeGroup := func() {
		if len(current) == 0 {
			return
		}
		groups = append(groups, newGroup(len(groups)+1, config, current))
		current = nil
		used = 0
	}
	for _, vessel := range vessels {
		if vessel.LengthM > config.EffectiveLengthM+fitTolerance {
			unplaceable = append(unplaceable, VesselOccupancy{Vessel: vessel, OccupancyM: vessel.LengthM})
			continue
		}
		addition := vessel.LengthM
		if len(current) > 0 {
			addition += config.SafetySpacingM
		}
		if len(current) > 0 && used+addition > config.EffectiveLengthM+fitTolerance {
			closeGroup()
			addition = vessel.LengthM
		}
		current = append(current, vessel)
		used += addition
	}
	closeGroup()
	return groups, unplaceable
}

func newGroup(sequence int, config Config, vessels []model.Vessel) Group {
	group := Group{Sequence: sequence, Vessels: make([]VesselOccupancy, 0, len(vessels))}
	for index, vessel := range vessels {
		spacing := 0.0
		if index < len(vessels)-1 {
			spacing = config.SafetySpacingM
		}
		group.Vessels = append(group.Vessels, VesselOccupancy{
			Vessel: vessel, SpacingM: spacing, OccupancyM: vessel.LengthM + spacing,
		})
		group.OccupancyM += vessel.LengthM + spacing
	}
	group.RemainingM = config.EffectiveLengthM - group.OccupancyM
	return group
}

// Occupancy reports the metres a single lockage group would occupy.
func Occupancy(config Config, vessels []model.Vessel) float64 {
	total := 0.0
	for index, vessel := range vessels {
		total += vessel.LengthM
		if index < len(vessels)-1 {
			total += config.SafetySpacingM
		}
	}
	return total
}

// Overflow describes a vessel set that cannot enter the chamber in one
// lockage. It names the specific vessels that fit and those excluded so the
// operator can adjust the convoy instead of receiving a truncated plan.
type Overflow struct {
	ChamberID        string            `json:"chamber_id"`
	EffectiveLengthM float64           `json:"effective_length_m"`
	SafetySpacingM   float64           `json:"safety_spacing_m"`
	RequiredM        float64           `json:"required_m"`
	ExcessM          float64           `json:"excess_m"`
	Fits             []VesselOccupancy `json:"fits"`
	Excluded         []VesselOccupancy `json:"excluded"`
}

func (e *Overflow) Error() string {
	names := make([]string, 0, len(e.Excluded))
	for _, item := range e.Excluded {
		names = append(names, fmt.Sprintf("%s (%.1fm)", item.Vessel.Name, item.OccupancyM))
	}
	return fmt.Sprintf(
		"stowage overflow in %s: %d vessel(s) require %.1fm but effective length is %.1fm (excess %.1fm); excluded: %s",
		e.ChamberID, len(e.Fits)+len(e.Excluded), e.RequiredM, e.EffectiveLengthM, e.ExcessM,
		strings.Join(names, ", "),
	)
}

// Check verifies that the whole vessel set fits one lockage in queue order.
// It returns nil when the convoy fits, or an *Overflow naming the vessels
// that exceed the chamber's effective length.
func Check(chamberID string, config Config, vessels []model.Vessel) error {
	groups, unplaceable := Layout(config, vessels)
	if len(unplaceable) == 0 && len(groups) <= 1 {
		return nil
	}
	overflow := &Overflow{
		ChamberID: chamberID, EffectiveLengthM: config.EffectiveLengthM,
		SafetySpacingM: config.SafetySpacingM, RequiredM: Occupancy(config, vessels),
	}
	overflow.ExcessM = overflow.RequiredM - config.EffectiveLengthM
	fits := make(map[uuid.UUID]struct{})
	if len(groups) > 0 {
		overflow.Fits = groups[0].Vessels
		for _, item := range groups[0].Vessels {
			fits[item.Vessel.ID] = struct{}{}
		}
	}
	occupancyByID := make(map[uuid.UUID]VesselOccupancy, len(unplaceable)+len(vessels))
	for _, item := range unplaceable {
		occupancyByID[item.Vessel.ID] = item
	}
	if len(groups) > 1 {
		for _, group := range groups[1:] {
			for _, item := range group.Vessels {
				occupancyByID[item.Vessel.ID] = item
			}
		}
	}
	for _, vessel := range vessels {
		if _, ok := fits[vessel.ID]; ok {
			continue
		}
		if item, ok := occupancyByID[vessel.ID]; ok {
			overflow.Excluded = append(overflow.Excluded, item)
		}
	}
	return overflow
}
