import type { MessageInitShape } from "@bufbuild/protobuf";
import { timestampDate } from "@bufbuild/protobuf/wkt";

import {
  GitFailure_Reason,
  ProbeResponse_Access,
  Strategy,
  type AuditEntry as AuditEntryMessage,
  type PublishedRepository as PublishedRepositoryMessage,
  type RevisionSchema,
  type WorkspaceResult,
} from "./gen/gitd/v1/gitd_pb.js";
import type { AuditEntry, Probe, PublishedRepository, StrategyName, Workspace } from "./types.js";

export function toWorkspace(path: string, result: WorkspaceResult): Workspace {
  const layers = result.layers;
  return {
    path,
    commit: result.commit,
    branch: result.branch,
    strategy: strategyName(result.strategy),
    layers: layers && { lower: layers.lower, upper: layers.upper, work: layers.work },
    pendingSubmodules: result.pendingSubmodules.map((submodule) => ({
      path: submodule.path,
      url: submodule.url,
      reason: submodule.failure?.gitSaid ?? "",
    })),
    lfsPatterns: result.lfsPatterns,
  };
}

export function strategyName(strategy: Strategy): StrategyName {
  switch (strategy) {
    case Strategy.OVERLAY:
      return "overlay";
    case Strategy.REFLINK:
      return "reflink";
    default:
      return "copy";
  }
}

export function strategyProto(name?: StrategyName): Strategy {
  switch (name) {
    case "overlay":
      return Strategy.OVERLAY;
    case "reflink":
      return Strategy.REFLINK;
    case "copy":
      return Strategy.COPY;
    default:
      return Strategy.UNSPECIFIED;
  }
}

export function accessName(access: ProbeResponse_Access): Probe["access"] {
  switch (access) {
    case ProbeResponse_Access.PUBLIC:
      return "public";
    case ProbeResponse_Access.REACHABLE:
      return "reachable";
    default:
      return "denied";
  }
}

/** The revision a prepare asks for: a commit, a branch, or the default branch. */
export function revisionOf(
  branch?: string,
  commit?: string,
): MessageInitShape<typeof RevisionSchema> | undefined {
  if (commit) {
    return { target: { case: "commit", value: commit } };
  }
  if (branch) {
    return { target: { case: "branch", value: branch } };
  }
  return undefined;
}

/** What a publish pushed: the workspace's repository first, then each submodule. */
export function pushedOf(
  repository: PublishedRepositoryMessage | undefined,
  submodules: PublishedRepositoryMessage[],
): PublishedRepository[] {
  return [repository, ...submodules]
    .filter((pushed) => pushed !== undefined)
    .map((pushed) => ({ url: pushed.url, path: pushed.path, commit: pushed.commit, subjects: pushed.subjects }));
}

export function toAuditEntry(entry: AuditEntryMessage): AuditEntry {
  const refusal = entry.refusal;
  return {
    time: entry.time ? timestampDate(entry.time) : new Date(0),
    workspace: entry.workspace,
    url: entry.url,
    branch: entry.branch,
    repositories: pushedOf(entry.repository, entry.submodules),
    refused: refusal && {
      reason: GitFailure_Reason[refusal.reason].replace("REASON_", ""),
      said: refusal.gitSaid,
    },
  };
}
