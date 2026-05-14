// L1: Branded ID types + smart constructors.
//
// Each ID is a Brand<string, "Tag"> at compile time. The smart constructor
// refines a raw string into the branded type, rejecting empty strings.
// Pass-through of a TurnID into a function expecting a PartID is a compile
// error — the brand discriminator differs even though both wrap string.

import { type Brand, unsafeBrand } from "../algebra/brand.js";
import { type Result, ok, err } from "../algebra/result.js";

export type SessionID = Brand<string, "SessionID">;
export type TurnID = Brand<string, "TurnID">;
export type PartID = Brand<string, "PartID">;
export type PermissionID = Brand<string, "PermissionID">;
export type SubAgentID = Brand<string, "SubAgentID">;
export type ToolCallID = Brand<string, "ToolCallID">;

export type IDParseError = {
  readonly kind: "id_empty" | "id_too_long";
  readonly raw: string;
  readonly tag: string;
};

const MAX_ID_LEN = 256;

function refine<T extends string>(
  raw: string,
  tag: T,
): Result<Brand<string, T>, IDParseError> {
  if (raw === "") return err({ kind: "id_empty", raw, tag });
  if (raw.length > MAX_ID_LEN) return err({ kind: "id_too_long", raw, tag });
  return ok(unsafeBrand<string, T>(raw));
}

export const sessionID = (raw: string): Result<SessionID, IDParseError> =>
  refine(raw, "SessionID");
export const turnID = (raw: string): Result<TurnID, IDParseError> =>
  refine(raw, "TurnID");
export const partID = (raw: string): Result<PartID, IDParseError> =>
  refine(raw, "PartID");
export const permissionID = (raw: string): Result<PermissionID, IDParseError> =>
  refine(raw, "PermissionID");
export const subAgentID = (raw: string): Result<SubAgentID, IDParseError> =>
  refine(raw, "SubAgentID");
export const toolCallID = (raw: string): Result<ToolCallID, IDParseError> =>
  refine(raw, "ToolCallID");
