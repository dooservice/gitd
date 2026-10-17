import type { Client } from "@connectrpc/connect";

import type { PublishingService } from "./gen/gitd/v1/gitd_pb.js";
import { call } from "./calls.js";
import { publishSettingsProto, toPublishSettings } from "./publishing-convert.js";
import type { PublishSettings } from "./publishing-types.js";

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
}
