import type { MessageInitShape } from "@bufbuild/protobuf";

import {
  Preset_Origin,
  PullRequestSettings_Open,
  type Preset as PresetMessage,
  type PresetSchema,
  type PublishSettings as PublishSettingsMessage,
  type PublishSettingsSchema,
  type PullRequestOptionsSchema,
} from "./gen/gitd/v1/gitd_pb.js";
import type { Preset, PublishSettings, PullRequestOptions } from "./publishing-types.js";

export function toPublishSettings(settings: PublishSettingsMessage | undefined): PublishSettings {
  const pullRequests = settings?.pullRequests;
  return {
    urlPrefix: settings?.urlPrefix ?? "",
    preset: settings?.preset || undefined,
    protectedPaths: settings?.protectedPaths ?? [],
    keptPaths: settings?.keptPaths ?? [],
    allowedPaths: settings?.allowedPaths ?? [],
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
    preset: settings.preset ?? "",
    protectedPaths: settings.protectedPaths ?? [],
    keptPaths: settings.keptPaths ?? [],
    allowedPaths: settings.allowedPaths ?? [],
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

export function toPreset(preset: PresetMessage | undefined): Preset {
  return {
    name: preset?.name ?? "",
    protectedPaths: preset?.protectedPaths ?? [],
    keptPaths: preset?.keptPaths ?? [],
    maxChangedFiles: preset?.maxChangedFiles,
    origin: originName(preset?.origin ?? Preset_Origin.UNSPECIFIED),
  };
}

export function presetProto(preset: Preset): MessageInitShape<typeof PresetSchema> {
  return {
    name: preset.name,
    protectedPaths: preset.protectedPaths ?? [],
    keptPaths: preset.keptPaths ?? [],
    maxChangedFiles: preset.maxChangedFiles,
  };
}

function originName(origin: Preset_Origin): Preset["origin"] {
  switch (origin) {
    case Preset_Origin.BUILT_IN:
      return "built_in";
    case Preset_Origin.FILE:
      return "file";
    default:
      return "api";
  }
}
