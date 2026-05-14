// L1: ModelChoice — provider + model id + optional credential reference.
//
// ModelChoice is a value type, immutable. The TUI never modifies a
// ModelChoice in place; switching models constructs a new one.

export type ModelChoice = {
  readonly provider: string;
  readonly model: string;
  readonly credentialKey?: string;
};

export function modelChoice(provider: string, model: string, credentialKey?: string): ModelChoice {
  return credentialKey === undefined
    ? { provider, model }
    : { provider, model, credentialKey };
}

export function modelLabel(m: ModelChoice): string {
  return `${m.provider}/${m.model}`;
}
