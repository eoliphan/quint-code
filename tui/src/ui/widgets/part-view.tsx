// L7: PartView — discriminated render by part.kind.
//
// Chat parts render inline; FPF artifact parts delegate to dedicated
// renderers (DecisionRecordView, ProblemCardView, etc.) which open
// the artifact in the FPF inspector route on click. The discriminator
// is exhaustive — adding a new Part kind is a compile error until a
// case is added here.

import { type JSX, Show } from "solid-js";
import type { Part } from "../../core/domain/part.js";
import { TextView } from "../primitives/text-view.js";
import { BoxView } from "../primitives/box-view.js";
import { Badge } from "../primitives/badge.js";
import { DecisionRecordView } from "./fpf/decision-record-view.js";
import { ProblemCardView } from "./fpf/problem-card-view.js";
import { SolutionPortfolioView } from "./fpf/solution-portfolio-view.js";
import { NoteView } from "./fpf/note-view.js";
import { EvidenceItemView } from "./fpf/evidence-item-view.js";
import { WorkCommissionView } from "./fpf/work-commission-view.js";

export interface PartViewProps {
  readonly part: Part;
  readonly onInspectArtifact?: (artifactId: string) => void;
}

export function PartView(props: PartViewProps): JSX.Element {
  const p = props.part;
  switch (p.kind) {
    case "text":
      return <TextView>{p.text}</TextView>;
    case "reasoning":
      return (
        <BoxView paddingLeft={2}>
          <TextView fg="fgMuted">
            ▶ reasoning ({truncate(p.text)})
          </TextView>
        </BoxView>
      );
    case "tool_use_started":
      return (
        <BoxView paddingLeft={2} paddingTop={1}>
          <TextView fg="toolName">
            ▸ {p.toolName} <Badge label={String(p.toolCallId)} />
          </TextView>
          <TextView fg="toolArgs">{stringifyArgs(p.args)}</TextView>
        </BoxView>
      );
    case "tool_use_completed":
      return (
        <BoxView paddingLeft={2}>
          <TextView fg={p.isError ? "toolError" : "toolResult"}>
            {p.isError ? "✗ " : "✓ "}
            {p.content}
          </TextView>
        </BoxView>
      );
    case "file_ref":
      return (
        <BoxView paddingLeft={2}>
          <TextView fg="fgMuted">📎 {p.path} ({p.mime}, {p.bytes}B)</TextView>
        </BoxView>
      );
    case "step_boundary":
      return (
        <BoxView paddingTop={1}>
          <TextView fg="fgDim">— {p.label} —</TextView>
        </BoxView>
      );
    case "decision_record":
      return (
        <Show when={true}>
          <DecisionRecordView part={p} onInspect={() => props.onInspectArtifact?.(p.artifactId)} />
        </Show>
      );
    case "problem_card":
      return (
        <ProblemCardView part={p} onInspect={() => props.onInspectArtifact?.(p.artifactId)} />
      );
    case "solution_portfolio":
      return (
        <SolutionPortfolioView part={p} onInspect={() => props.onInspectArtifact?.(p.artifactId)} />
      );
    case "note":
      return <NoteView part={p} onInspect={() => props.onInspectArtifact?.(p.artifactId)} />;
    case "evidence_item":
      return (
        <EvidenceItemView part={p} onInspect={() => props.onInspectArtifact?.(p.artifactId)} />
      );
    case "work_commission":
      return (
        <WorkCommissionView part={p} onInspect={() => props.onInspectArtifact?.(p.artifactId)} />
      );
    default: {
      const _exhaustive: never = p;
      void _exhaustive;
      return <TextView fg="danger">unknown part kind</TextView>;
    }
  }
}

function stringifyArgs(args: unknown): string {
  try {
    return JSON.stringify(args, null, 2);
  } catch {
    return String(args);
  }
}

function truncate(s: string): string {
  return s.length > 60 ? `${s.slice(0, 57)}…` : s;
}
