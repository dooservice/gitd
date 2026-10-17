import type { Client } from "@connectrpc/connect";

import type { PublishingService } from "./gen/gitd/v1/gitd_pb.js";
import { call } from "./calls.js";
import { presetProto, publishSettingsProto, toPreset, toPublishSettings } from "./publishing-convert.js";
import type { Preset, PublishSettings } from "./publishing-types.js";

/**
 * The rules publishing follows, per repository prefix, as `gitd.publishing`: for a
 * platform's own pages, never for an agent.
 */
export class Publishing {
  constructor(private readonly publishing: Client<typeof PublishingService>) {}

  /** Every setting, gitd.toml's included. */
  async settings(): Promise<PublishSettings[]> {
    const answer = await call(() => this.publishing.getSettings({}));
    return answer.settings.map(toPublishSettings);
  }

  /** Keeps the settings of a prefix, replacing those it had, as gitd checked them. */
  async set(settings: PublishSettings): Promise<PublishSettings> {
    const answer = await call(() => this.publishing.setSettings({ settings: publishSettingsProto(settings) }));
    return toPublishSettings(answer.settings);
  }

  /** Forgets the settings of a prefix set through the API. */
  async remove(urlPrefix: string): Promise<void> {
    await call(() => this.publishing.removeSettings({ urlPrefix }));
  }

  /** Every preset as it applies: gitd's own, gitd.toml's and those set through the API. */
  async presets(): Promise<Preset[]> {
    const answer = await call(() => this.publishing.listPresets({}));
    return answer.presets.map(toPreset);
  }

  /** Keeps a preset, replacing one of the same name; `recommended` replaces gitd's own. */
  async setPreset(preset: Preset): Promise<Preset> {
    const answer = await call(() => this.publishing.setPreset({ preset: presetProto(preset) }));
    return toPreset(answer.preset);
  }

  /** Forgets a preset set through the API; refused while settings take it. */
  async removePreset(name: string): Promise<void> {
    await call(() => this.publishing.removePreset({ name }));
  }
}
