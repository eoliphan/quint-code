package reff

import (
	"math"
	"testing"
)

const calibTol = 1e-9

func closeEnough(got, want float64) bool {
	return math.Abs(got-want) <= calibTol
}

func TestBrierScore_KnownPoints(t *testing.T) {
	cases := []struct {
		p       float64
		outcome bool
		want    float64
	}{
		{1.0, true, 0.0},   // perfect confident hit
		{0.0, false, 0.0},  // perfect confident miss
		{1.0, false, 1.0},  // maximally wrong
		{0.0, true, 1.0},   // maximally wrong
		{0.5, true, 0.25},  // maximally uncertain
		{0.5, false, 0.25}, // symmetric
	}
	for _, c := range cases {
		got := BrierScore(c.p, c.outcome)
		if !closeEnough(got, c.want) {
			t.Errorf("BrierScore(%v, %v) = %v, want %v", c.p, c.outcome, got, c.want)
		}
	}
	// Out-of-range probabilities clamp, never panic or exceed [0,1] error.
	if got := BrierScore(1.7, false); !closeEnough(got, 1.0) {
		t.Errorf("BrierScore clamp: got %v, want 1.0", got)
	}
}

// TestDecompose_MurphyReference checks the decomposition against a hand-computed
// reference set, and the Murphy (1973) identity MeanBrier = Reliability −
// Resolution + Uncertainty. Set (p, outcome): three forecasts at p=1.0 with
// outcomes {1,1,0}, two at p=0.5 with outcomes {1,0}.
//
//	N=5, base rate ō=3/5=0.6
//	Uncertainty = 0.6·0.4                 = 0.24
//	bin 1.0: n=3, ō=2/3 → 3·(1−2/3)²      = 3·(1/9)   = 1/3
//	bin 0.5: n=2, ō=1/2 → 2·(0.5−0.5)²    = 0
//	Reliability = (1/3)/5                  = 1/15  ≈ 0.0666667
//	Resolution  = (3·(2/3−0.6)² + 2·(0.5−0.6)²)/5 = (3/450 + 0.02)/5 = 1/150 ≈ 0.0066667
//	MeanBrier   = (0+0+1+0.25+0.25)/5      = 0.3
func TestDecompose_MurphyReference(t *testing.T) {
	fs := []Forecast{
		{1.0, true}, {1.0, true}, {1.0, false},
		{0.5, true}, {0.5, false},
	}
	c := Decompose(fs)

	if c.N != 5 {
		t.Fatalf("N = %d, want 5", c.N)
	}
	checks := []struct {
		name      string
		got, want float64
	}{
		{"BaseRate", c.BaseRate, 0.6},
		{"Uncertainty", c.Uncertainty, 0.24},
		{"Reliability", c.Reliability, 1.0 / 15.0},
		{"Resolution", c.Resolution, 1.0 / 150.0},
		{"MeanBrier", c.MeanBrier, 0.3},
		{"MeanForecast", c.MeanForecast, 0.8},
	}
	for _, ck := range checks {
		if !closeEnough(ck.got, ck.want) {
			t.Errorf("%s = %v, want %v", ck.name, ck.got, ck.want)
		}
	}

	// The Murphy identity must hold exactly within float tolerance.
	identity := c.Reliability - c.Resolution + c.Uncertainty
	if !closeEnough(identity, c.MeanBrier) {
		t.Errorf("Murphy identity broken: REL−RES+UNC = %v, MeanBrier = %v", identity, c.MeanBrier)
	}
}

// TestDecompose_IdentityHolds fuzzes a few unrelated sets and asserts the Murphy
// identity each time — the decomposition is meaningless if the identity drifts.
func TestDecompose_IdentityHolds(t *testing.T) {
	sets := [][]Forecast{
		{{0.9, true}, {0.9, true}, {0.9, false}, {0.2, false}, {0.2, true}, {0.7, true}},
		{{0.1, false}, {0.3, false}, {0.6, true}, {0.6, false}, {1.0, true}},
		{{0.5, true}}, // singleton
		{{0.4, false}, {0.4, false}, {0.4, false}},
	}
	for i, fs := range sets {
		c := Decompose(fs)
		identity := c.Reliability - c.Resolution + c.Uncertainty
		if !closeEnough(identity, c.MeanBrier) {
			t.Errorf("set %d: identity %v != MeanBrier %v", i, identity, c.MeanBrier)
		}
	}
}

func TestDecompose_Empty(t *testing.T) {
	c := Decompose(nil)
	if c.N != 0 || c.MeanBrier != 0 {
		t.Errorf("empty decompose should be zero, got %+v", c)
	}
}

// TestCoherentizePartition_RelatedClaims confirms coherentization bites on a set
// of logically-related (partition) claims: three mutually-exclusive probabilities
// summing to 1.2 get pulled to sum 1 by the minimal additive shift.
func TestCoherentizePartition_RelatedClaims(t *testing.T) {
	in := []float64{0.5, 0.4, 0.3} // sum 1.2 — incoherent partition
	out := CoherentizePartition(in)

	sum := 0.0
	for _, p := range out {
		sum += p
	}
	if !closeEnough(sum, 1.0) {
		t.Errorf("coherentized partition should sum to 1, got %v (%v)", sum, out)
	}
	// Every member shifted by exactly (1−1.2)/3 = −0.0666667.
	shift := -0.2 / 3.0
	for i := range in {
		if !closeEnough(out[i], in[i]+shift) {
			t.Errorf("member %d = %v, want %v", i, out[i], in[i]+shift)
		}
	}
}

// TestCoherentizePartition_SingletonDegradesToClamp confirms the weakest-link
// behavior: with no partner claim there is no partition constraint, so the
// operation degenerates to a clamp rather than forcing the lone probability to 1.
func TestCoherentizePartition_SingletonDegradesToClamp(t *testing.T) {
	cases := []struct {
		in   []float64
		want []float64
	}{
		{[]float64{1.5}, []float64{1.0}},  // clamp high, NOT forced to its own sum
		{[]float64{-0.2}, []float64{0.0}}, // clamp low
		{[]float64{0.7}, []float64{0.7}},  // already valid, untouched
	}
	for _, c := range cases {
		out := CoherentizePartition(c.in)
		if len(out) != len(c.want) || !closeEnough(out[0], c.want[0]) {
			t.Errorf("CoherentizePartition(%v) = %v, want %v", c.in, out, c.want)
		}
	}
}

func TestAssessCalibration_Direction(t *testing.T) {
	tol := 0.1

	// Forecasts run high (mean 0.9) but only 1 of 4 outcomes true → overconfident.
	over := AssessCalibration([]Forecast{
		{0.9, true}, {0.9, false}, {0.9, false}, {0.9, false},
	}, tol)
	if over.Direction != Overconfident {
		t.Errorf("expected overconfident, got %s (bias %v)", over.Direction, over.Bias)
	}

	// Forecasts run low (mean 0.1) but 3 of 4 outcomes true → underconfident.
	under := AssessCalibration([]Forecast{
		{0.1, true}, {0.1, true}, {0.1, true}, {0.1, false},
	}, tol)
	if under.Direction != Underconfident {
		t.Errorf("expected underconfident, got %s (bias %v)", under.Direction, under.Bias)
	}

	// Mean forecast matches base rate → calibrated.
	cal := AssessCalibration([]Forecast{
		{0.5, true}, {0.5, false}, {0.5, true}, {0.5, false},
	}, tol)
	if cal.Direction != Calibrated {
		t.Errorf("expected calibrated, got %s (bias %v)", cal.Direction, cal.Bias)
	}
}
