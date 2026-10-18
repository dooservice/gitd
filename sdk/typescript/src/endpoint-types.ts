/** A token of gitd's endpoint as gitd shows it, never its secret. */
export interface TokenInfo {
  id: string;
  /** The repository it reads. */
  url: string;
  /** The sandbox repository it pushes to, when writable. */
  sandbox: string;
  writable: boolean;
  issuedAt: Date;
  expiresAt: Date;
}

/** A token just issued: its secret, shown this once, and where the sandbox clones from. */
export interface IssuedToken {
  secret: string;
  cloneUrl: string;
  /** The clone URL with the secret in it, for the sandbox's git: `git clone <this>`. */
  authenticatedUrl: string;
  info: TokenInfo;
}

export interface IssueTokenOptions {
  /** The branch the sandbox starts from; default the repository's default branch. */
  branch?: string;
  /** How long the token lives, in seconds: an hour by default, a day at most. */
  lifetimeSeconds?: number;
  readOnly?: boolean;
  /** A sandbox repository from an earlier token, to keep pushing to. */
  sandbox?: string;
}
