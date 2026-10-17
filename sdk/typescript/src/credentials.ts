import type { Client } from "@connectrpc/connect";

import type { ConnectionService } from "./gen/gitd/v1/gitd_pb.js";
import { call } from "./calls.js";
import { toKeptCredential } from "./credential-convert.js";
import type {
  AddGitHubAppOptions,
  AppToCreate,
  BeginGitHubAppOptions,
  Connected,
  KeptCredential,
  SignIn,
} from "./credential-types.js";

/**
 * The credentials gitd keeps, as `gitd.credentials`: for a platform's own pages, never for
 * an agent.
 */
export class Credentials {
  constructor(private readonly connections: Client<typeof ConnectionService>) {}

  /** Every credential gitd keeps, gitd.toml's included, never a secret. */
  async list(): Promise<KeptCredential[]> {
    const answer = await call(() => this.connections.listConnections({}));
    return answer.connections.map(toKeptCredential);
  }

  /** Forgets a credential kept through the API. */
  async remove(id: string): Promise<void> {
    await call(() => this.connections.removeConnection({ id }));
  }

  /** Keeps a token for the repositories under urlPrefix, which names the host. */
  async addToken(urlPrefix: string, token: string, user = ""): Promise<KeptCredential> {
    const answer = await call(() => this.connections.addToken({ urlPrefix, token, user }));
    return toKeptCredential(answer.connection);
  }

  /** Keeps a GitHub App a platform already has, once GitHub takes the key as its own. */
  async addGitHubApp(options: AddGitHubAppOptions): Promise<KeptCredential> {
    const answer = await call(() =>
      this.connections.addGitHubApp({
        appId: BigInt(options.appId),
        privateKey: options.privateKey,
        host: options.host ?? "",
        urlPrefix: options.urlPrefix ?? "",
      }),
    );
    return toKeptCredential(answer.connection);
  }

  /** Starts creating the organisation's own GitHub App from a manifest. */
  async beginGitHubApp(options: BeginGitHubAppOptions): Promise<AppToCreate> {
    const answer = await call(() =>
      this.connections.beginGitHubApp({
        name: options.name,
        redirectUrl: options.redirectUrl,
        organization: options.organization ?? "",
        host: options.host ?? "",
      }),
    );
    return { formAction: answer.formAction, manifest: answer.manifest, state: answer.state };
  }

  /** Finishes creating the App with the code and state GitHub sent back. */
  async completeGitHubApp(code: string, state: string): Promise<Connected> {
    const answer = await call(() => this.connections.completeGitHubApp({ code, state }));
    return { credential: toKeptCredential(answer.connection), installUrl: answer.installUrl };
  }

  /** Asks GitHub for the code a person enters to sign in through gitd's public App. */
  async startGitHubLogin(host = ""): Promise<SignIn> {
    const answer = await call(() => this.connections.startGitHubLogin({ host }));
    return {
      userCode: answer.userCode,
      verificationUrl: answer.verificationUrl,
      loginId: answer.loginId,
      expiresInSeconds: answer.expiresInSeconds,
    };
  }

  /** Waits until the person has entered the code, and keeps the sign-in. */
  async finishGitHubLogin(loginId: string): Promise<Connected> {
    const answer = await call(() => this.connections.finishGitHubLogin({ loginId }));
    return { credential: toKeptCredential(answer.connection), installUrl: answer.installUrl };
  }
}
