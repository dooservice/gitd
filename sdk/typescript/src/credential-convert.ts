import { timestampDate } from "@bufbuild/protobuf/wkt";

import { Connection_Kind, type Connection } from "./gen/gitd/v1/gitd_pb.js";
import type { CredentialKind, KeptCredential } from "./credential-types.js";

export function toKeptCredential(connection: Connection | undefined): KeptCredential {
  return {
    id: connection?.id ?? "",
    kind: kindName(connection?.kind ?? Connection_Kind.UNSPECIFIED),
    urlPrefix: connection?.urlPrefix ?? "",
    actsAs: connection?.actsAs ?? "",
    appId: Number(connection?.appId ?? 0),
    manageUrl: connection?.manageUrl ?? "",
    expires: connection?.expires && timestampDate(connection.expires),
    fromFile: connection?.fromFile ?? false,
    created: connection?.created && timestampDate(connection.created),
  };
}

function kindName(kind: Connection_Kind): CredentialKind {
  switch (kind) {
    case Connection_Kind.SSH_KEY:
      return "ssh_key";
    case Connection_Kind.GITHUB_APP:
      return "github_app";
    case Connection_Kind.GITHUB_LOGIN:
      return "github_login";
    default:
      return "token";
  }
}
