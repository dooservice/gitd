/** A ready set of rules: protects the CI, CODEOWNERS and .gitmodules, keeps tests, allows 200 files. */
export type Preset = "recommended";

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
  preset?: Preset;
  /** Paths a publish may not change, as git's glob pathspecs, e.g. `.github/workflows/**`. */
  protectedPaths?: string[];
  /** Paths a publish may change but not delete or rename away. */
  keptPaths?: string[];
  maxChangedFiles?: number;
  /** What every published branch must start with, e.g. `agents/`. */
  branchPrefix?: string;
  pullRequests?: PullRequestSettings;
  /** Declared in gitd.toml: shown, never changed through the API. */
  fromFile?: boolean;
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

export interface PublishOptions {
  /** Who asked for the work, as their account on the host. */
  requestedBy?: string;
  pullRequest?: PullRequestOptions;
}

/** A pull request gitd opened. */
export interface OpenedPullRequest {
  url: string;
  number: number;
}
