import type { WarmLayer as WarmLayerMessage } from "./gen/gitd/v1/gitd_pb.js";
import type { WarmLayer } from "./types.js";

/** A warm layer as gitd answers it, read as this client's. */
export function toWarmLayer(warm: WarmLayerMessage | undefined): WarmLayer {
  return {
    url: warm?.url ?? "",
    name: warm?.name ?? "",
    commit: warm?.commit ?? "",
    sealedAt: new Date(Number(warm?.sealedAt ?? 0n) * 1000),
    sealedFrom: warm?.sealedFrom ?? "",
    changedFiles: warm?.changedFiles ?? 0,
    users: warm?.users ?? 0,
  };
}
