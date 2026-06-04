package artifact

import (
	"context"
	"fmt"

	"github.com/m0n0x41d/haft/internal/reff"
)

// CalibrationCalibratedBand is the half-width of the "calibrated" band on the
// mean-forecast-vs-base-rate gap. Outside it, the read names an over- or
// under-confidence bias.
const CalibrationCalibratedBand = 0.1

// CalibrationMinForecasts is the floor below which a calibration profile is
// statistically meaningless (cold-start). Surfaces gate on this before treating
// the directional bias as actionable (dec-20260603-c3c7fa88 weakest link).
const CalibrationMinForecasts = 15

// forecastOutcome maps a verified claim status to a binary calibration outcome.
// Only supported/refuted carry a clean binary signal; weakened, inconclusive and
// unverified are not forecasts yet and return ok=false.
func forecastOutcome(status ClaimStatus) (outcome bool, ok bool) {
	switch status {
	case ClaimStatusSupported:
		return true, true
	case ClaimStatusRefuted:
		return false, true
	default:
		return false, false
	}
}

// forecastsFromClaims extracts the (probability, realized-outcome) pairs from a
// set of decision claims. A claim contributes a forecast only when it carries an
// elicited probability AND has reached a terminal binary status. Pure.
func forecastsFromClaims(claims []DecisionClaim) []reff.Forecast {
	out := make([]reff.Forecast, 0, len(claims))
	for _, c := range claims {
		if c.Probability == nil {
			continue
		}
		outcome, ok := forecastOutcome(c.Status)
		if !ok {
			continue
		}
		out = append(out, reff.Forecast{Probability: *c.Probability, Outcome: outcome})
	}
	return out
}

// CalibrationProfile scans every decision record, collects each verified forecast
// (a claim with an elicited probability and a terminal binary status), and returns
// the decomposed-Brier calibration verdict. The only side effect is reading the
// store; all scoring is the pure reff core. Reading the profile is off the hot
// path — it is invoked at /h-verify time, never on a mid-task query
// (dec-20260603-c3c7fa88).
func CalibrationProfile(ctx context.Context, store ArtifactStore) (reff.CalibrationVerdict, error) {
	// ListByKind returns lightweight heads without structured_data; Get hydrates
	// the claims. The N+1 read is acceptable — this runs at /h-verify, off the
	// hot path, never on a mid-task query.
	heads, err := store.ListByKind(ctx, KindDecisionRecord, 0)
	if err != nil {
		return reff.CalibrationVerdict{}, err
	}
	forecasts := make([]reff.Forecast, 0)
	for _, h := range heads {
		full, err := store.Get(ctx, h.Meta.ID)
		if err != nil {
			return reff.CalibrationVerdict{}, fmt.Errorf("load decision %s for calibration: %w", h.Meta.ID, err)
		}
		forecasts = append(forecasts, forecastsFromClaims(full.UnmarshalDecisionFields().Claims)...)
	}
	return reff.AssessCalibration(forecasts, CalibrationCalibratedBand), nil
}
