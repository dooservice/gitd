/** What a credential gitd keeps is: a token, an SSH key, a GitHub App, or a person signed in. */
export type CredentialKind = "token" | "ssh_key" | "github_app" | "github_login";

/** One credential gitd keeps, never its secret. */
export interface KeptCredential {
  /** What to remove it by. */
  id: string;
  kind: CredentialKind;
  /** The repositories it opens: those whose address starts with this. */
  urlPrefix: string;
  /** Whom it acts as: an App's bot (`acme-agents[bot]`) or a person. */
  actsAs: string;
  appId: number;
  /** Where an App's installations are managed. */
  manageUrl: string;
  /** When a sign-in stops renewing. */
  expires?: Date;
  /** Declared in gitd.toml: shown, never changed through the API. */
  fromFile: boolean;
  created?: Date;
}

/** A GitHub App or sign-in gitd now keeps, and where it is installed on repositories. */
export interface Connected {
  credential: KeptCredential;
  installUrl: string;
}

/** What a browser posts to GitHub to create an App: `manifest`, as a form field, to `formAction`. */
export interface AppToCreate {
  formAction: string;
  manifest: string;
  /** Sent back by GitHub with the code, for completeGitHubApp. */
  state: string;
}

/** A sign-in waiting for a person to enter `userCode` at `verificationUrl`. */
export interface SignIn {
  userCode: string;
  verificationUrl: string;
  loginId: string;
  expiresInSeconds: number;
}

export interface BeginGitHubAppOptions {
  /** The App's name on GitHub; its bot is `<name>[bot]`. */
  name: string;
  /** Where GitHub sends the person back with a code and the state. */
  redirectUrl: string;
  /** The organisation that owns it; default: the person creating it. */
  organization?: string;
  /** A GitHub Enterprise Server's host; default github.com. */
  host?: string;
}

export interface AddGitHubAppOptions {
  appId: number;
  /** The App's private key, in PEM. */
  privateKey: string;
  host?: string;
  /** The repositories it opens; default every one on the host. */
  urlPrefix?: string;
}
