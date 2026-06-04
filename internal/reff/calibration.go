package reff

import (
	"math"
	"sort"
)

// Forecast pairs an elicited probability with the realized binary outcome of the
// claim it was attached to. Outcome is true when the claim was SUPPORTED at
// verification, false when REFUTED. Weakened / inconclusive claims carry no clean
// binary outcome and are excluded by the caller before scoring.
type Forecast struct {
	Probability float64 // elicited p(claim holds), in [0,1]
	Outcome     bool    // realized: supported (true) / refuted (false)
}

// BrierComponents is the Murphy (1973) three-term decomposition of the mean
// Brier score over a set of probabilistic forecasts:
//
//	MeanBrier = Reliability − Resolution + Uncertainty
//
// Reliability is calibration error (LOWER is better — forecast probabilities
// match observed frequencies). Resolution is discrimination (HIGHER is better —
// forecasts separate outcomes from the base rate). Uncertainty is the intrinsic
// difficulty of the task (base-rate variance), independent of the forecaster.
type BrierComponents struct {
	N            int
	MeanBrier    float64
	Reliability  float64
	Resolution   float64
	Uncertainty  float64
	BaseRate     float64 // observed frequency of outcome=true
	MeanForecast float64 // mean elicited probability
}

// Clamp constrains a probability to the valid [0,1] range.
func Clamp(p float64) float64 {
	return math.Max(0, math.Min(1, p))
}

// BrierScore is the squared error of a single probabilistic forecast: (p − o)²,
// with o = 1 when the outcome occurred and 0 otherwise. 0 is a perfect forecast,
// 1 is maximally wrong.
func BrierScore(p float64, outcome bool) float64 {
	o := 0.0
	if outcome {
		o = 1.0
	}
	d := Clamp(p) - o
	return d * d
}

// MeanBrier is the average Brier score over a set of forecasts. Zero forecasts
// score 0 (no signal, not a perfect score — callers gate on N before reading).
func MeanBrier(fs []Forecast) float64 {
	if len(fs) == 0 {
		return 0
	}
	sum := 0.0
	for _, f := range fs {
		sum += BrierScore(f.Probability, f.Outcome)
	}
	return sum / float64(len(fs))
}

// forecastBin accumulates the within-bin forecast mass and positive outcomes for
// one group of equal-valued forecasts.
type forecastBin struct {
	count    int
	sumP     float64
	positive int
}

// Decompose computes the Murphy (1973) reliability–resolution–uncertainty
// decomposition by grouping forecasts into bins of equal forecast value. Binning
// by distinct value keeps each bin's mean equal to the forecast itself, so the
// identity MeanBrier = Reliability − Resolution + Uncertainty holds exactly
// (within float tolerance). Pure and deterministic — bins are summed in sorted
// key order so the result never depends on input order or map iteration.
func Decompose(fs []Forecast) BrierComponents {
	n := len(fs)
	if n == 0 {
		return BrierComponents{}
	}

	bins := map[int]*forecastBin{}
	positives := 0
	sumP := 0.0
	for _, f := range fs {
		p := Clamp(f.Probability)
		key := int(math.Round(p * 1000)) // stable integer key; true mean kept below
		b := bins[key]
		if b == nil {
			b = &forecastBin{}
			bins[key] = b
		}
		b.count++
		b.sumP += p
		sumP += p
		if f.Outcome {
			b.positive++
			positives++
		}
	}

	baseRate := float64(positives) / float64(n)
	uncertainty := baseRate * (1 - baseRate)

	keys := make([]int, 0, len(bins))
	for k := range bins {
		keys = append(keys, k)
	}
	sort.Ints(keys)

	reliability := 0.0
	resolution := 0.0
	for _, k := range keys {
		b := bins[k]
		nk := float64(b.count)
		fk := b.sumP / nk              // within-bin mean forecast
		ok := float64(b.positive) / nk // within-bin observed frequency
		reliability += nk * (fk - ok) * (fk - ok)
		resolution += nk * (ok - baseRate) * (ok - baseRate)
	}
	reliability /= float64(n)
	resolution /= float64(n)

	return BrierComponents{
		N:            n,
		MeanBrier:    MeanBrier(fs),
		Reliability:  reliability,
		Resolution:   resolution,
		Uncertainty:  uncertainty,
		BaseRate:     baseRate,
		MeanForecast: sumP / float64(n),
	}
}

// CoherentizePartition makes a set of probabilities asserted to PARTITION one
// outcome space (mutually exclusive, jointly exhaustive) mutually coherent by the
// minimal-L2 additive projection onto the hyperplane Σp = 1, then clamps each to
// [0,1]. This is the linear coherence adjustment whose calibration gain the
// research reports on logically-related claims. A set of fewer than two
// probabilities carries no partition constraint, so it degenerates to a clamp —
// coherentization only bites on related claims, never on an isolated forecast.
func CoherentizePartition(ps []float64) []float64 {
	out := make([]float64, len(ps))
	if len(ps) < 2 {
		for i, p := range ps {
			out[i] = Clamp(p)
		}
		return out
	}
	sum := 0.0
	for _, p := range ps {
		sum += p
	}
	shift := (1 - sum) / float64(len(ps))
	for i, p := range ps {
		out[i] = Clamp(p + shift)
	}
	return out
}

// CalibrationDirection labels the systematic bias in a set of verified forecasts.
type CalibrationDirection string

const (
	Calibrated     CalibrationDirection = "calibrated"
	Overconfident  CalibrationDirection = "overconfident"  // forecasts run higher than outcomes warrant
	Underconfident CalibrationDirection = "underconfident" // forecasts run lower than outcomes warrant
)

// CalibrationVerdict is the operator-facing read over a set of verified forecasts:
// the Murphy components plus a directional bias derived from mean-forecast vs the
// realized base rate. The agent's elicited probability is one noisy vote — this
// read only becomes meaningful once enough verified forecasts accumulate, so the
// caller gates on Components.N before acting on Direction.
type CalibrationVerdict struct {
	Components BrierComponents
	Direction  CalibrationDirection
	Bias       float64 // MeanForecast − BaseRate
}

// directionFor maps a forecast-vs-outcome gap to a labeled bias, with tol the
// half-width of the calibrated band.
func directionFor(bias, tol float64) CalibrationDirection {
	if bias > tol {
		return Overconfident
	}
	if bias < -tol {
		return Underconfident
	}
	return Calibrated
}

// AssessCalibration decomposes the forecasts and labels the directional bias.
// tol is the half-width of the "calibrated" band on (MeanForecast − BaseRate).
func AssessCalibration(fs []Forecast, tol float64) CalibrationVerdict {
	c := Decompose(fs)
	bias := c.MeanForecast - c.BaseRate
	return CalibrationVerdict{
		Components: c,
		Direction:  directionFor(bias, tol),
		Bias:       bias,
	}
}
