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
import { ToolDot } from "../primitives/tool-dot.js";
import { DiffView, looksLikeDiff } from "./diff-view.js";
import { MarkdownView } from "./markdown-view.js";
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
      return <MarkdownView text={p.text} />;
    case "reasoning":
      return (
        <BoxView paddingLeft={2}>
          <TextView fg="fgDim">
            ▶ reasoning <span style={{ fg: "#666" }}>{`(${p.text.length} chars)`}</span>
          </TextView>
          <BoxView paddingLeft={2}>
            <TextView fg="fgDim">{truncate(p.text, 200)}</TextView>
          </BoxView>
        </BoxView>
      );
    case "tool_use_started":
      return (
        <BoxView paddingLeft={1} paddingTop={1} flexDirection="row">
          <ToolDot state="running" />
          <BoxView>
            <TextView fg="toolName">
              <b>{p.toolName}</b>
              <span style={{ fg: "#888" }}> ({oneLineArgs(p.args)})</span>
            </TextView>
          </BoxView>
        </BoxView>
      );
    case "tool_use_completed":
      return (
        <BoxView paddingLeft={1} flexDirection="row">
          <ToolDot state={p.isError ? "error" : "ok"} />
          <BoxView flexGrow={1}>
            <TextView fg={p.isError ? "toolError" : "toolName"}>
              <b>{p.toolName}</b>
            </TextView>
            <Show
              when={looksLikeDiff(p.content)}
              fallback={
                <TextView fg={p.isError ? "toolError" : "fgDim"}>{truncateContent(p.content)}</TextView>
              }
            >
              <BoxView paddingLeft={2}>
                <DiffView diff={p.content} />
              </BoxView>
            </Show>
          </BoxView>
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

function oneLineArgs(args: unknown): string {
  if (args === null || args === undefined) return "";
  let serialised: string;
  try {
    serialised = JSON.stringify(args);
  } catch {
    serialised = String(args);
  }
  // Tool-call argument blob, single-line. Long values truncate so
  // the chat surface stays readable; the FPF inspector route can
  // show the full payload when the operator needs it.
  return truncate(serialised, 80);
}

function truncate(s: string, max: number = 60): string {
  return s.length > max ? `${s.slice(0, max - 1)}…` : s;
}

function truncateContent(s: string): string {
  // tool_use_completed content can be huge (full file reads, bash
  // output). Cap at ~600 chars in the chat surface; operators
  // press the inspect shortcut to see the full payload.
  if (s.length <= 600) return s;
  return s.slice(0, 600) + `\n… (${s.length - 600} more chars)`;
}
