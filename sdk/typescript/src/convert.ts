import type { MessageInitShape } from "@bufbuild/protobuf";
import { timestampDate } from "@bufbuild/protobuf/wkt";

import {
  ChangedFile_Kind,
  ConfigurationChange_Action,
  GitFailure_Reason,
  ProbeResponse_Access,
  Strategy,
  type AuditEntry as AuditEntryMessage,
  type ChangedFile as ChangedFileMessage,
  type ChangesResponse,
  type GitFailure,
  type OpenedPullRequest as OpenedPullRequestMessage,
  type PublishResponse,
  type PublishedRepository as PublishedRepositoryMessage,
  type RevisionSchema,
  type WorkspaceResult,
} from "./gen/gitd/v1/gitd_pb.js";
import type { OpenedPullRequest } from "./publishing-types.js";
import type {
  AuditEntry,
  ChangedFile,
  ChangeKind,
  Changes,
  Probe,
  Published,
  PublishedRepository,
  Refusal,
  StrategyName,
  Workspace,
} from "./types.js";

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

/** What a publish pushed, and the pull request it opened or why it could not. */
export function publishedOf(answer: PublishResponse): Published {
  return {
    branch: answer.branch,
    repositories: pushedOf(answer.repository, answer.submodules),
    pullRequest: openedOf(answer.pullRequest),
    pullRequestRefused: refusalOf(answer.pullRequestFailure),
  };
}

export function toAuditEntry(entry: AuditEntryMessage): AuditEntry {
  const change = entry.change;
  return {
    time: entry.time ? timestampDate(entry.time) : new Date(0),
    workspace: entry.workspace,
    url: entry.url,
    branch: entry.branch,
    repositories: pushedOf(entry.repository, entry.submodules),
    refused: refusalOf(entry.refusal),
    requestedBy: entry.requestedBy,
    pullRequest: openedOf(entry.pullRequest),
    pullRequestRefused: refusalOf(entry.pullRequestFailure),
    provenance: entry.provenance && {
      agent: entry.provenance.agent,
      model: entry.provenance.model,
      session: entry.provenance.session,
      taskUrl: entry.provenance.taskUrl,
    },
    change: change && {
      action: (ConfigurationChange_Action[change.action] ?? "").replace("ACTION_", "").toLowerCase(),
      subject: change.subject,
      urlPrefix: change.urlPrefix,
    },
  };
}

function openedOf(opened: OpenedPullRequestMessage | undefined): OpenedPullRequest | undefined {
  return opened && { url: opened.url, number: Number(opened.number) };
}

function refusalOf(failure: GitFailure | undefined): Refusal | undefined {
  return failure && { reason: (GitFailure_Reason[failure.reason] ?? "").replace("REASON_", ""), said: failure.gitSaid };
}

export function toChanges(answer: ChangesResponse): Changes {
  return { base: answer.base, fromLayer: answer.fromLayer, files: toChangedFiles(answer.files) };
}

export function toChangedFiles(files: ChangedFileMessage[]): ChangedFile[] {
  return files.map((file) => ({
    path: file.path,
    kind: kindName(file.kind),
    addedLines: file.addedLines,
    removedLines: file.removedLines,
    binary: file.binary,
  }));
}

function kindName(kind: ChangedFile_Kind): ChangeKind {
  switch (kind) {
    case ChangedFile_Kind.ADDED:
      return "added";
    case ChangedFile_Kind.DELETED:
      return "deleted";
    default:
      return "modified";
  }
}
