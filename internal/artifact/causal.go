package artifact

import (
	"fmt"
	"strings"
)

// CausalEvidenceSupportBasis is the C.28 controlled vocabulary for the epistemic
// basis on which an EvidenceItem supports a causal-use claim. Per CC-B3.9 an
// undeclared basis cannot raise R for a causal-ladder climb; the canonical
// strings here are the only admissible values for new evidence.
type CausalEvidenceSupportBasis string

const (
	CausalBasisObservational          CausalEvidenceSupportBasis = "observationalAssociationSupportBasis"
	CausalBasisInterventional         CausalEvidenceSupportBasis = "interventionalActionSupportBasis"
	CausalBasisRealizedCounterfactual CausalEvidenceSupportBasis = "realizedCounterfactualSampleSupportBasis"
	CausalBasisIdentifiedEstimate     CausalEvidenceSupportBasis = "identifiedCounterfactualEstimateSupportBasis"
	CausalBasisSimulationOnly         CausalEvidenceSupportBasis = "simulationOnlyCounterfactualOutputBasis"
)

// RealizabilityVerdict tracks the C.28 CounterfactualSamplingRealizabilityProfile
// verdict associated with a DecisionClaim. Empty means the claim is not asserted
// as a causal-use claim and therefore has no realizability verdict.
type RealizabilityVerdict string

const (
	RealizabilityRealizable    RealizabilityVerdict = "realizable"
	RealizabilityNonrealizable RealizabilityVerdict = "nonrealizable"
	RealizabilityUnknown       RealizabilityVerdict = "unknown"
)

// ParseCausalSupportBasis canonicalizes operator-friendly aliases into the
// long C.28 vocabulary strings. Empty input is allowed and returns "".
func ParseCausalSupportBasis(raw string) (CausalEvidenceSupportBasis, error) {
	key := normalizeCausalKey(raw)
	switch key {
	case "":
		return "", nil
	case "observational", "observationalassociation", "observationalassociationsupportbasis":
		return CausalBasisObservational, nil
	case "interventional", "interventionalaction", "interventionalactionsupportbasis":
		return CausalBasisInterventional, nil
	case "realizedcounterfactual", "realizedcounterfactualsample", "realizedcounterfactualsamplesupportbasis":
		return CausalBasisRealizedCounterfactual, nil
	case "identifiedestimate", "identifiedcounterfactual", "identifiedcounterfactualestimate", "identifiedcounterfactualestimatesupportbasis":
		return CausalBasisIdentifiedEstimate, nil
	case "simulationonly", "simulation", "simulationonlycounterfactualoutput", "simulationonlycounterfactualoutputbasis":
		return CausalBasisSimulationOnly, nil
	default:
		return "", fmt.Errorf(
			"causal_support_basis must be one of observational|interventional|realized_counterfactual|identified_estimate|simulation_only (got %q)",
			raw,
		)
	}
}

// ParseRealizabilityVerdict canonicalizes realizability inputs. Empty input is
// allowed and returns "". The C.28 vocabulary is closed at three values.
func ParseRealizabilityVerdict(raw string) (RealizabilityVerdict, error) {
	key := normalizeCausalKey(raw)
	switch key {
	case "":
		return "", nil
	case "realizable":
		return RealizabilityRealizable, nil
	case "nonrealizable", "notrealizable", "unrealizable":
		return RealizabilityNonrealizable, nil
	case "unknown":
		return RealizabilityUnknown, nil
	default:
		return "", fmt.Errorf("realizability must be one of realizable|nonrealizable|unknown (got %q)", raw)
	}
}

func normalizeCausalKey(raw string) string {
	key := strings.ToLower(strings.TrimSpace(raw))
	key = strings.ReplaceAll(key, "_", "")
	key = strings.ReplaceAll(key, "-", "")
	key = strings.ReplaceAll(key, " ", "")
	return key
}
