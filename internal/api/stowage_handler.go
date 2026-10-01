package api

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wyw14/cry-96/internal/model"
	"github.com/wyw14/cry-96/internal/stowage"
)

func stowageHandler(runtime *Runtime) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		result := make(map[string]stowage.Status, len(runtime.Chambers))
		for _, chamberID := range runtime.Chambers {
			result[chamberID] = stowageStatus(runtime, chamberID)
		}
		writeJSON(w, http.StatusOK, result)
	}
}

type stowageConfigInput struct {
	EffectiveLengthM float64 `json:"effective_length_m"`
	SafetySpacingM   float64 `json:"safety_spacing_m"`
}

func configureStowageHandler(runtime *Runtime) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		chamberID := chi.URLParam(r, "chamberID")
		if !chamberKnown(runtime, chamberID) {
			writeError(w, http.StatusNotFound, errors.New("unknown chamber"))
			return
		}
		var input stowageConfigInput
		if err := decodeJSON(r, &input); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		_, err := runtime.Stowage.Configure(r.Context(), chamberID, stowage.Config{
			EffectiveLengthM: input.EffectiveLengthM, SafetySpacingM: input.SafetySpacingM,
		})
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, stowageStatus(runtime, chamberID))
	}
}

type stowagePreviewInput struct {
	ChamberID string        `json:"chamber_id"`
	Vessels   []vesselInput `json:"vessels"`
}

func stowagePreviewHandler(runtime *Runtime) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input stowagePreviewInput
		if err := decodeJSON(r, &input); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if !chamberKnown(runtime, input.ChamberID) {
			writeError(w, http.StatusNotFound, errors.New("unknown chamber"))
			return
		}
		vessels, err := buildVessels(input.Vessels)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		groups, unplaceable := runtime.Stowage.Groups(input.ChamberID, vessels)
		config, _ := runtime.Stowage.Config(input.ChamberID)
		writeJSON(w, http.StatusOK, map[string]any{
			"config": config, "groups": groups, "unplaceable": unplaceable,
			"fits_single_lockage": len(unplaceable) == 0 && len(groups) <= 1,
		})
	}
}

func stowageStatus(runtime *Runtime, chamberID string) stowage.Status {
	entries := runtime.AdmissionQueue.List(chamberID)
	queued := make([]model.Vessel, 0, len(entries))
	for _, entry := range entries {
		if !entry.Released {
			queued = append(queued, entry.Vessel)
		}
	}
	return runtime.Stowage.Status(chamberID, runtime.Planner.Active(chamberID), queued)
}

func planStowageSummary(runtime *Runtime, plan model.ConvoyPlan) map[string]any {
	config, _ := runtime.Stowage.Config(plan.ChamberID)
	occupancy := stowage.Occupancy(config, plan.Vessels)
	return map[string]any{
		"effective_length_m": config.EffectiveLengthM,
		"safety_spacing_m":   config.SafetySpacingM,
		"occupancy_m":        occupancy,
		"remaining_m":        config.EffectiveLengthM - occupancy,
	}
}

func chamberKnown(runtime *Runtime, chamberID string) bool {
	for _, known := range runtime.Chambers {
		if known == chamberID {
			return true
		}
	}
	return false
}

func buildVessels(items []vesselInput) ([]model.Vessel, error) {
	vessels := make([]model.Vessel, 0, len(items))
	for _, item := range items {
		vessel, err := model.NewVessel(item.Name, item.CallSign, item.LengthM)
		if err != nil {
			return nil, err
		}
		vessels = append(vessels, vessel)
	}
	return vessels, nil
}
