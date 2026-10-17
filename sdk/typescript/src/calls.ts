import { Code } from "@connectrpc/connect";

import type { Progress, WorkspaceResult } from "./gen/gitd/v1/gitd_pb.js";
import { GitdError, refusal } from "./errors.js";

/** A message of a workspace stream: progress, or the result that ends it. */
type Event = {
  event:
    | { case: "progress"; value: Progress }
    | { case: "result"; value: WorkspaceResult }
    | { case: undefined; value?: undefined };
};

/** Reads a stream to its result, handing the progress on. */
export async function drain(
  stream: AsyncIterable<Event>,
  onProgress?: (progress: Progress) => void,
): Promise<WorkspaceResult> {
  try {
    let result: WorkspaceResult | undefined;
    for await (const message of stream) {
      if (message.event.case === "progress") {
        onProgress?.(message.event.value);
      } else if (message.event.case === "result") {
        result = message.event.value;
      }
    }
    if (!result) {
      throw new GitdError("", "gitd ended the call without a result", Code.Internal);
    }
    return result;
  } catch (error) {
    throw refusal(error);
  }
}

/** Makes a unary call, reading a refusal into a GitdError. */
export async function call<T>(request: () => Promise<T>): Promise<T> {
  try {
    return await request();
  } catch (error) {
    throw refusal(error);
  }
}
