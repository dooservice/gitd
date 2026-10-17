import type { MessageInitShape } from "@bufbuild/protobuf";
import { timestampDate } from "@bufbuild/protobuf/wkt";

import {
  WatchResponse_Kind,
  type Side as SideMessage,
  type SideSchema,
  type WatchResponse,
} from "./gen/gitd/v1/gitd_pb.js";
import type { Side, WorkspaceEvent } from "./types.js";

export function sideProto(side: Side): MessageInitShape<typeof SideSchema> {
  return "workspace" in side
    ? { of: { case: "workspace", value: side.workspace } }
    : { of: { case: "branch", value: side.branch } };
}

export function toSide(side: SideMessage | undefined): Side {
  return side?.of.case === "workspace" ? { workspace: side.of.value } : { branch: side?.of.value ?? "" };
}

export function toEvent(event: WatchResponse): WorkspaceEvent {
  return {
    kind: eventKind(event.kind),
    path: event.path,
    time: event.time ? timestampDate(event.time) : new Date(0),
    branch: event.branch,
    detail: event.detail,
  };
}

function eventKind(kind: WatchResponse_Kind): WorkspaceEvent["kind"] {
  switch (kind) {
    case WatchResponse_Kind.MADE:
      return "made";
    case WatchResponse_Kind.PUBLISHED:
      return "published";
    case WatchResponse_Kind.REFUSED:
      return "refused";
    case WatchResponse_Kind.REMOVED:
      return "removed";
    default:
      return "changed";
  }
}
