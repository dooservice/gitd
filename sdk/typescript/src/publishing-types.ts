/** When publishes open pull requests: only when a publish asks, or always. */
export type Opening = "when_asked" | "always";

/** How publishes open pull requests. */
export interface PullRequestSettings {
  opening: Opening;
  draft?: boolean;
  labels?: string[];
  /** Ask the person a publish names as requester to review it. */
  requestReviewFromRequester?: boolean;
}

/** The rules of the repositories whose address starts with urlPrefix ("" for every one). */
export interface PublishSettings {
  urlPrefix: string;
  /** The name of a preset whose rules these take; `recommended` is gitd's own. */
  preset?: string;
  /** Paths a publish may not change, as git's glob pathspecs, e.g. `.github/workflows/**`. */
  protectedPaths?: string[];
  /** Paths a publish may change but not delete or rename away. */
  keptPaths?: string[];
  /** Paths these settings' own rules leave free, e.g. `.gitmodules` under `recommended`. */
  allowedPaths?: string[];
  maxChangedFiles?: number;
  /** What every published branch must start with, e.g. `agents/`. */
  branchPrefix?: string;
  pullRequests?: PullRequestSettings;
  /** Declared in gitd.toml: shown, never changed through the API. */
  fromFile?: boolean;
}

/** Where a preset comes from: gitd's own, gitd.toml, or the API. */
export type PresetOrigin = "built_in" | "file" | "api";

/** A named set of rules settings take; one named `recommended` replaces gitd's own. */
export interface Preset {
  name: string;
  protectedPaths?: string[];
  keptPaths?: string[];
  maxChangedFiles?: number;
  /** Read from gitd, never sent. */
  origin?: PresetOrigin;
}

/** A publish's own say on its pull request. */
export interface PullRequestOptions {
  /** Open one even when the settings open them only when asked. */
  open?: boolean;
  /** Default: the newest commit's subject. */
  title?: string;
  /** Text that comes before what gitd checked. */
  body?: string;
  /** The branch it merges into; default: the repository's default branch. */
  base?: string;
  draft?: boolean;
}

/** Where a publish's work came from, as the platform says; gitd records it as told. */
export interface Provenance {
  agent?: string;
  model?: string;
  session?: string;
  /** An https address: an issue, a ticket. */
  taskUrl?: string;
}

export interface PublishOptions {
  /** Who asked for the work, as their account on the host. */
  requestedBy?: string;
  pullRequest?: PullRequestOptions;
  provenance?: Provenance;
}

/** A pull request gitd opened. */
export interface OpenedPullRequest {
  url: string;
  number: number;
}
