// L8: FPFInspectorRoute — drill-down view for a single FPF artifact.
//
// Scans the current Session's turns + parts for the artifact ID and
// renders the appropriate FPF view at full fidelity. Closing returns
// to the agent.session route.

import { type JSX, Show } from "solid-js";
import { TextView } from "../primitives/text-view.js";
import { BoxView } from "../primitives/box-view.js";
import { KeyHintBar } from "../primitives/key-hint-bar.js";
import { Divider } from "../primitives/divider.js";
import { DecisionRecordView } from "../widgets/fpf/decision-record-view.js";
import { ProblemCardView } from "../widgets/fpf/problem-card-view.js";
import { SolutionPortfolioView } from "../widgets/fpf/solution-portfolio-view.js";
import { NoteView } from "../widgets/fpf/note-view.js";
import { EvidenceItemView } from "../widgets/fpf/evidence-item-view.js";
import { WorkCommissionView } from "../widgets/fpf/work-commission-view.js";
import type { Session } from "../../core/domain/session.js";
import { turns } from "../../core/domain/session.js";
import type { Part } from "../../core/domain/part.js";
import type { Action } from "../../effect/state/actions.js";
import type { ToastEntry } from "../../effect/state/toast-store.js";
import { ToastStack } from "../widgets/toast-stack.js";

export interface FPFInspectorRouteProps {
  readonly session: () => Session | undefined;
  readonly artifactId: () => string | undefined;
  readonly toasts: () => ReadonlyArray<ToastEntry>;
  readonly hints: () => ReadonlyArray<{ readonly key: string; readonly action: string }>;
  readonly dispatch: (action: Action) => void;
}

export function FPFInspectorRoute(props: FPFInspectorRouteProps): JSX.Element {
  const part = (): Part | undefined => {
    const s = props.session();
    const aid = props.artifactId();
    if (s === undefined || aid === undefined) return undefined;
    for (const t of turns(s)) {
      for (const p of t.parts) {
        if (
          p.kind === "decision_record" ||
          p.kind === "problem_card" ||
          p.kind === "solution_portfolio" ||
          p.kind === "note" ||
          p.kind === "evidence_item" ||
          p.kind === "work_commission"
        ) {
          if (p.artifactId === aid) return p;
        }
      }
    }
    return undefined;
  };

  return (
    <BoxView paddingLeft={1} paddingRight={1}>
      <TextView fg="toolName">▣ FPF Inspector</TextView>
      <Divider />
      <Show
        when={part()}
        fallback={
          <BoxView>
            <TextView fg="fgMuted">artifact not found in current session</TextView>
            <TextView fg="fgDim">id: {props.artifactId() ?? "(none)"}</TextView>
          </BoxView>
        }
      >
        {(p) => {
          const cur = p();
          if (cur.kind === "decision_record") return <DecisionRecordView part={cur} />;
          if (cur.kind === "problem_card") return <ProblemCardView part={cur} />;
          if (cur.kind === "solution_portfolio") return <SolutionPortfolioView part={cur} />;
          if (cur.kind === "note") return <NoteView part={cur} />;
          if (cur.kind === "evidence_item") return <EvidenceItemView part={cur} />;
          if (cur.kind === "work_commission") return <WorkCommissionView part={cur} />;
          return <TextView fg="danger">unknown artifact kind</TextView>;
        }}
      </Show>
      <Divider />
      <TextView
        fg="toolName"
        onMouseUp={() => props.dispatch({ tag: "NavigateRoute", to: "agent.session" })}
      >
        [esc] back to session
      </TextView>
      <ToastStack entries={props.toasts} />
      <KeyHintBar hints={props.hints} />
    </BoxView>
  );
}
