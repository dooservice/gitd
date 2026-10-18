import { timestampDate } from "@bufbuild/protobuf/wkt";
import type { Client } from "@connectrpc/connect";

import type { EndpointService, TokenInfo as TokenInfoMessage } from "./gen/gitd/v1/gitd_pb.js";
import { call } from "./calls.js";
import type { IssuedToken, IssueTokenOptions, TokenInfo } from "./endpoint-types.js";

/**
 * gitd's endpoint, as `gitd.endpoint`: tokens for sandboxes that clone over HTTP instead of
 * mounting a workspace. What a sandbox pushes is published with `gitd.publish("", branch,
 * { sandbox })`.
 */
export class Endpoint {
  constructor(private readonly endpoint: Client<typeof EndpointService>) {}

  /** A token for a sandbox: it reads url and pushes to its own sandbox repository. */
  async issue(url: string, options: IssueTokenOptions = {}): Promise<IssuedToken> {
    const answer = await call(() =>
      this.endpoint.issueToken({
        url,
        branch: options.branch ?? "",
        lifetimeSeconds: BigInt(options.lifetimeSeconds ?? 0),
        readOnly: options.readOnly ?? false,
        sandbox: options.sandbox ?? "",
      }),
    );
    const [scheme, rest] = answer.cloneUrl.split("://", 2);
    return {
      secret: answer.token,
      cloneUrl: answer.cloneUrl,
      authenticatedUrl: rest ? `${scheme}://x:${answer.token}@${rest}` : answer.cloneUrl,
      info: toTokenInfo(answer.info),
    };
  }

  /** Ends a token now, and removes its sandbox repository when asked. */
  async revoke(id: string, removeSandbox = false): Promise<void> {
    await call(() => this.endpoint.revokeToken({ id, removeSandbox }));
  }

  /** The tokens alive, never their secrets. */
  async tokens(): Promise<TokenInfo[]> {
    const answer = await call(() => this.endpoint.listTokens({}));
    return answer.tokens.map(toTokenInfo);
  }
}

function toTokenInfo(info: TokenInfoMessage | undefined): TokenInfo {
  return {
    id: info?.id ?? "",
    url: info?.url ?? "",
    sandbox: info?.sandbox ?? "",
    writable: info?.writable ?? false,
    issuedAt: info?.issuedAt ? timestampDate(info.issuedAt) : new Date(0),
    expiresAt: info?.expiresAt ? timestampDate(info.expiresAt) : new Date(0),
  };
}
