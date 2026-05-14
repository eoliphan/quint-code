// L1: Permission with typestate.
//
// `Permission<'pending'>` and `Permission<'resolved'>` are distinct types.
// A function that runs a tool MUST receive a `Permission<'resolved'>`
// with `decision === 'approved'`; passing a pending one is a compile
// error, and passing a denied one is rejected by a value-level check
// the type narrowing reveals.

import { type PermissionID, type ToolCallID, type TurnID } from "./ids.js";

export type PermissionState = "pending" | "resolved";

type PermissionBase<S extends PermissionState> = {
  readonly id: PermissionID;
  readonly turnId: TurnID;
  readonly toolCallId: ToolCallID;
  readonly toolName: string;
  readonly args: unknown;
  readonly requestedAt: Date;
  readonly state: S;
};

export type PermissionPending = PermissionBase<"pending">;

export type PermissionResolved = PermissionBase<"resolved"> & {
  readonly decision: "approved" | "denied";
  readonly reason: string;
  readonly resolvedAt: Date;
};

export type Permission<S extends PermissionState = PermissionState> =
  S extends "pending" ? PermissionPending :
  S extends "resolved" ? PermissionResolved :
  PermissionPending | PermissionResolved;

export function isPending(p: Permission): p is PermissionPending {
  return p.state === "pending";
}

export function isResolved(p: Permission): p is PermissionResolved {
  return p.state === "resolved";
}

export function isApproved(p: PermissionResolved): boolean {
  return p.decision === "approved";
}
