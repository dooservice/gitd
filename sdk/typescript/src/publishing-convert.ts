import type { MessageInitShape } from "@bufbuild/protobuf";

import {
  Preset as PresetMessage,
  PullRequestSettings_Open,
  type PublishSettings as PublishSettingsMessage,
  type PublishSettingsSchema,
  type PullRequestOptionsSchema,
} from "./gen/gitd/v1/gitd_pb.js";
import type { PublishSettings, PullRequestOptions } from "./publishing-types.js";

export function toPublishSettings(settings: PublishSettingsMessage | undefined): PublishSettings {
  const pullRequests = settings?.pullRequests;
  return {
    urlPrefix: settings?.urlPrefix ?? "",
    preset: settings?.preset === PresetMessage.RECOMMENDED ? "recommended" : undefined,
    protectedPaths: settings?.protectedPaths ?? [],
    keptPaths: settings?.keptPaths ?? [],
    maxChangedFiles: settings?.maxChangedFiles,
    branchPrefix: settings?.branchPrefix ?? "",
    pullRequests: pullRequests && {
      opening: pullRequests.open === PullRequestSettings_Open.ALWAYS ? "always" : "when_asked",
      draft: pullRequests.draft,
      labels: pullRequests.labels,
      requestReviewFromRequester: pullRequests.requestReviewFromRequester,
    },
    fromFile: settings?.fromFile ?? false,
  };
}

export function publishSettingsProto(settings: PublishSettings): MessageInitShape<typeof PublishSettingsSchema> {
  const pullRequests = settings.pullRequests;
  return {
    urlPrefix: settings.urlPrefix,
    preset: settings.preset === "recommended" ? PresetMessage.RECOMMENDED : PresetMessage.UNSPECIFIED,
    protectedPaths: settings.protectedPaths ?? [],
    keptPaths: settings.keptPaths ?? [],
    maxChangedFiles: settings.maxChangedFiles,
    branchPrefix: settings.branchPrefix ?? "",
    pullRequests: pullRequests && {
      open: pullRequests.opening === "always" ? PullRequestSettings_Open.ALWAYS : PullRequestSettings_Open.WHEN_ASKED,
      draft: pullRequests.draft ?? false,
      labels: pullRequests.labels ?? [],
      requestReviewFromRequester: pullRequests.requestReviewFromRequester ?? false,
    },
  };
}

export function pullRequestOptionsProto(
  options: PullRequestOptions | undefined,
): MessageInitShape<typeof PullRequestOptionsSchema> | undefined {
  return (
    options && {
      open: options.open ?? false,
      title: options.title ?? "",
      body: options.body ?? "",
      base: options.base ?? "",
      draft: options.draft,
    }
  );
}
