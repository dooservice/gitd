import type { Progress } from "./gen/gitd/v1/gitd_pb.js";
import type { OpenedPullRequest } from "./publishing-types.js";

/** How a workspace was made: a layer on a shared template, a block clone, or every file written. */
export type StrategyName = "overlay" | "reflink" | "copy";

/** Where a workspace ended up. */
export interface Workspace {
  path: string;
  commit: string;
  /** The branch it follows; empty when detached at a commit. */
  branch: string;
  strategy: StrategyName;
  /** An overlay workspace's template (lower) and its own layer (upper, work). */
  layers?: Layers;
  /** Submodules that could not be checked out, and why. */
  pendingSubmodules: PendingSubmodule[];
  /** Files the repository keeps in git LFS: pointers here, as gitd does not download them. */
  lfsPatterns: string[];
}

export interface Layers {
  lower: string;
  upper: string;
  work: string;
}

export interface PendingSubmodule {
  path: string;
  url: string;
  reason: string;
}

/** What a workspace is on disk. */
export interface Description {
  url: string;
  commit: string;
  branch: string;
  /** Whether there are uncommitted changes. */
  modified: boolean;
  strategy: StrategyName;
  submodules: Submodule[];
  /** Files kept in git LFS, which are pointers here. */
  lfsPatterns: string[];
}

export interface Submodule {
  path: string;
  url: string;
  commit: string;
  checkedOut: boolean;
}

/** Why gitd refused something: its reason (POLICY_REFUSED, AUTH_REQUIRED, …) and what was said. */
export interface Refusal {
  reason: string;
  said: string;
}

/**
 * What a publish pushed: the workspace's repository first, then each submodule; the pull
 * request it opened, or why it could not, the branch being published either way.
 */
export interface Published {
  branch: string;
  repositories: PublishedRepository[];
  pullRequest?: OpenedPullRequest;
  pullRequestRefused?: Refusal;
}

export interface PublishedRepository {
  url: string;
  /** The submodule's path; empty for the workspace's own repository. */
  path: string;
  commit: string;
  /** The subjects of the new commits, newest first. */
  subjects: string[];
}

/** Which gitd answers, and how it makes workspaces on its machine. */
export interface Info {
  version: string;
  gitVersion: string;
  pools: number;
  poolsBytes: number;
  templates: number;
  templatesBytes: number;
  strategy: StrategyName;
  strategyReason: string;
}

/** Whether a repository can be read, and its branches. */
export interface Probe {
  access: "public" | "reachable" | "denied";
  defaultBranch: string;
  /** Each branch and the commit it points at. */
  branches: Record<string, string>;
  /** Why it cannot be read, when denied. */
  denied?: string;
}

/** A change to the credentials or publishing settings, as the audit log keeps it. */
export interface ConfigurationChange {
  /** connection_added, connection_removed, settings_set or settings_removed. */
  action: string;
  /** The credential's id, or the settings' prefix. */
  subject: string;
  urlPrefix: string;
}

/** One publish, done or refused, or one change to credentials or settings, as the audit log keeps it. */
export interface AuditEntry {
  time: Date;
  /** The workspace's path, as the caller gave it. */
  workspace: string;
  /** The workspace's repository; empty when the workspace could not be opened. */
  url: string;
  branch: string;
  /** What was pushed, the workspace's repository first; empty when refused. */
  repositories: PublishedRepository[];
  /** Why it was refused, when it was. */
  refused?: Refusal;
  /** Who asked for the work, as the publish said. */
  requestedBy: string;
  pullRequest?: OpenedPullRequest;
  pullRequestRefused?: Refusal;
  change?: ConfigurationChange;
}

export interface AuditOptions {
  /** Only the publishes from this time on. */
  since?: Date;
  /** Only the publishes of this repository, by its URL. */
  url?: string;
  /** At most this many: 100 by default, never more than 1000. */
  limit?: number;
}

/** Options of a call that streams progress. */
export interface Progressing {
  /** Told what git is doing while the call runs. */
  onProgress?: (progress: Progress) => void;
  signal?: AbortSignal;
}

export interface PrepareOptions extends Progressing {
  url: string;
  path: string;
  branch?: string;
  commit?: string;
  submodules?: boolean;
  fullHistory?: boolean;
  /** A preference; gitd falls back to copy when it cannot run here. */
  strategy?: StrategyName;
}

export interface UpdateOptions extends Progressing {
  /** The branch to follow; default: the one it follows. */
  branch?: string;
  submodules?: boolean;
}

export interface CheckoutOptions extends Progressing {
  submodules?: boolean;
}
