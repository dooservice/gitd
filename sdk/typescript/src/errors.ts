import { Code, ConnectError } from "@connectrpc/connect";

import { GitFailureSchema, GitFailure_Reason } from "./gen/gitd/v1/gitd_pb.js";

/**
 * A refusal from gitd: its reason (AUTH_REQUIRED, NOT_FOUND, PATH_REFUSED, BUSY, …) and
 * what git said. Tell users what happened from the reason, never by parsing the message.
 */
export class GitdError extends Error {
  constructor(
    readonly reason: string,
    readonly said: string,
    readonly code: Code,
  ) {
    super(reason ? `${reason}: ${said}` : said);
    this.name = "GitdError";
  }
}

/** Reads a Connect error into a GitdError; anything else is passed on as it is. */
export function refusal(error: unknown): unknown {
  if (error instanceof GitdError || !(error instanceof ConnectError)) {
    return error;
  }
  const [failure] = error.findDetails(GitFailureSchema);
  if (!failure) {
    return new GitdError("", error.rawMessage, error.code);
  }
  const reason = GitFailure_Reason[failure.reason].replace("REASON_", "");
  return new GitdError(reason, failure.gitSaid, error.code);
}
