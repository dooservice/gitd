import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import { createClient, type Client, type Interceptor, type Transport } from "@connectrpc/connect";
import { readFileSync } from "node:fs";
import { createConnectTransport } from "@connectrpc/connect-node";

import {
  ConnectionService,
  EndpointService,
  PublishingService,
  RepositoryService,
  ServerService,
  WorkspaceService,
} from "./gen/gitd/v1/gitd_pb.js";
import { call, drain } from "./calls.js";
import { sideProto, toEvent, toSide } from "./compare-convert.js";
import { toWarmLayer } from "./warm-convert.js";
import { refusal } from "./errors.js";
import { Credentials } from "./credentials.js";
import { Endpoint } from "./endpoint.js";
import { pullRequestOptionsProto } from "./publishing-convert.js";
import { Publishing } from "./publishing.js";
import type { PublishOptions } from "./publishing-types.js";
import {
  accessName,
  publishedOf,
  revisionOf,
  strategyName,
  strategyProto,
  toAuditEntry,
  toChangedFiles,
  toChanges,
  toWorkspace,
} from "./convert.js";
import type {
  AuditEntry,
  AuditOptions,
  Changes,
  CheckoutOptions,
  CompareOptions,
  Comparison,
  Conflict,
  Description,
  Diff,
  DiffOptions,
  Info,
  PrepareOptions,
  Probe,
  Published,
  Side,
  WorkspaceEvent,
  UpdateOptions,
  WarmLayer,
  Workspace,
} from "./types.js";

export interface GitdOptions {
  /** gitd's Unix socket; default $GITD_SOCKET, then /run/gitd/gitd.sock. */
  socket?: string;
  /**
   * A gitd on the network instead, e.g. `https://gitd.internal:9420` (or http:// on a private
   * network); default $GITD_URL.
   */
  url?: string;
  /** The key naming your tenant; default the file $GITD_KEY_FILE names, then $GITD_KEY. */
  key?: string;
  /** The PEM of a certificate authority the system does not trust; default $GITD_CA_FILE's. */
  ca?: string;
  /** A transport of your own instead, e.g. for tests. */
  transport?: Transport;
}

const defaultSocket = "/run/gitd/gitd.sock";

/**
 * A gitd on its Unix socket. No call on workspaces takes a credential: gitd reads and
 * publishes with the ones it keeps, which a platform manages through `credentials`, and
 * publishing follows the rules it sets through `publishing`.
 */
export class Gitd {
  /** The credentials gitd keeps, and connecting GitHub. */
  readonly credentials: Credentials;
  /** The rules publishing follows, per repository prefix. */
  readonly publishing: Publishing;
  /** Tokens of gitd's endpoint, for sandboxes elsewhere. */
  readonly endpoint: Endpoint;
  private readonly workspaces: Client<typeof WorkspaceService>;
  private readonly repositories: Client<typeof RepositoryService>;
  private readonly server: Client<typeof ServerService>;

  constructor(options: GitdOptions = {}) {
    const transport = options.transport ?? transportFor(options);
    this.workspaces = createClient(WorkspaceService, transport);
    this.repositories = createClient(RepositoryService, transport);
    this.server = createClient(ServerService, transport);
    this.credentials = new Credentials(createClient(ConnectionService, transport));
    this.publishing = new Publishing(createClient(PublishingService, transport));
    this.endpoint = new Endpoint(createClient(EndpointService, transport));
  }

  /** Which gitd answers, and how it makes workspaces here and why. */
  async info(): Promise<Info> {
    const answer = await call(() => this.server.info({}));
    return {
      version: answer.version,
      gitVersion: answer.gitVersion,
      pools: answer.pools,
      poolsBytes: Number(answer.poolsBytes),
      templates: answer.templates,
      templatesBytes: Number(answer.templatesBytes),
      strategy: strategyName(answer.strategy),
      strategyReason: answer.strategyReason,
    };
  }

  /** Whether a repository can be read with the credentials gitd keeps, and its branches. */
  async probe(url: string): Promise<Probe> {
    const answer = await call(() => this.repositories.probe({ url }));
    return {
      access: accessName(answer.access),
      defaultBranch: answer.defaultBranch,
      branches: { ...answer.commits },
      denied: answer.failure?.gitSaid,
    };
  }

  /** Makes a workspace of a repository at a path under gitd's roots. */
  async prepare(options: PrepareOptions): Promise<Workspace> {
    const request = {
      url: options.url,
      path: options.path,
      revision: revisionOf(options.branch, options.commit),
      withSubmodules: options.submodules ?? false,
      fullHistory: options.fullHistory ?? false,
      strategy: strategyProto(options.strategy),
      warm: options.warm ?? "",
    };
    const stream = this.workspaces.prepare(request, { signal: options.signal });
    return toWorkspace(options.path, await drain(stream, options.onProgress));
  }

  /** Brings a workspace to the tip of its branch, or of the branch given. */
  async update(path: string, options: UpdateOptions = {}): Promise<Workspace> {
    const request = { path, branch: options.branch ?? "", withSubmodules: options.submodules ?? false };
    const stream = this.workspaces.update(request, { signal: options.signal });
    return toWorkspace(path, await drain(stream, options.onProgress));
  }

  /** Moves a workspace to a commit, as a rollback does. */
  async checkout(path: string, commit: string, options: CheckoutOptions = {}): Promise<Workspace> {
    const request = { path, commit, withSubmodules: options.submodules ?? false };
    const stream = this.workspaces.checkout(request, { signal: options.signal });
    return toWorkspace(path, await drain(stream, options.onProgress));
  }

  /** What a workspace is on disk, without the network. */
  async describe(path: string): Promise<Description> {
    const answer = await call(() => this.workspaces.describe({ path }));
    return {
      url: answer.url,
      commit: answer.commit,
      branch: answer.branch,
      modified: answer.modified,
      strategy: strategyName(answer.strategy),
      submodules: answer.submodules.map((submodule) => ({
        path: submodule.path,
        url: submodule.url,
        commit: submodule.commit,
        checkedOut: submodule.checkedOut,
      })),
      lfsPatterns: answer.lfsPatterns,
    };
  }

  /**
   * Pushes what was committed in a workspace to a new branch, with the credential gitd
   * keeps, and opens its pull request when the publishing settings or the options say so.
   */
  async publish(path: string, branch: string, options: PublishOptions = {}): Promise<Published> {
    const answer = await call(() =>
      this.workspaces.publish({
        path,
        branch,
        requestedBy: options.requestedBy ?? "",
        sandbox: options.sandbox ?? "",
        pullRequest: pullRequestOptionsProto(options.pullRequest),
        provenance: options.provenance && {
          agent: options.provenance.agent ?? "",
          model: options.provenance.model ?? "",
          session: options.provenance.session ?? "",
          taskUrl: options.provenance.taskUrl ?? "",
        },
      }),
    );
    return publishedOf(answer);
  }

  /** The audit log: every publish, done or refused, newest first. */
  async audit(options: AuditOptions = {}): Promise<AuditEntry[]> {
    const request = {
      since: options.since && timestampFromDate(options.since),
      url: options.url ?? "",
      limit: options.limit ?? 0,
    };
    const answer = await call(() => this.server.audit(request));
    return answer.entries.map(toAuditEntry);
  }

  /** What was changed in a workspace since gitd put it on its commit, committed or not. */
  async changes(path: string, includeIgnored = false): Promise<Changes> {
    return toChanges(await call(() => this.workspaces.changes({ path, includeIgnored })));
  }

  /** The unified diff of a workspace's changes, of some files or all, cut short at a size. */
  async diff(path: string, options: DiffOptions = {}): Promise<Diff> {
    const answer = await call(() =>
      this.workspaces.diff({
        path,
        files: options.files ?? [],
        maxBytes: BigInt(options.maxBytes ?? 0),
        includeIgnored: options.includeIgnored ?? false,
      }),
    );
    return { base: answer.base, diff: answer.diff, truncated: answer.truncated };
  }

  /** How two works of one repository differ, from first to second. */
  async compare(first: Side, second: Side, options: CompareOptions = {}): Promise<Comparison> {
    const answer = await call(() =>
      this.workspaces.compare({
        first: sideProto(first),
        second: sideProto(second),
        url: options.url ?? "",
        withDiff: options.withDiff ?? false,
        maxBytes: BigInt(options.maxBytes ?? 0),
        includeIgnored: options.includeIgnored ?? false,
      }),
    );
    return { files: toChangedFiles(answer.files), diff: answer.diff, truncated: answer.truncated };
  }

  /** Which pairs of works do not merge cleanly; a workspace stands for what it committed. */
  async conflicts(sides: Side[], url = ""): Promise<Conflict[]> {
    const answer = await call(() => this.workspaces.conflicts({ sides: sides.map(sideProto), url }));
    return answer.conflicts.map((conflict) => ({
      first: toSide(conflict.first),
      second: toSide(conflict.second),
      paths: conflict.paths,
    }));
  }

  /** What happens to the workspaces at paths, or to every one, as it happens. */
  async *watch(paths: string[] = [], signal?: AbortSignal): AsyncGenerator<WorkspaceEvent> {
    try {
      for await (const event of this.workspaces.watch({ paths }, { signal })) {
        yield toEvent(event);
      }
    } catch (error) {
      if (signal?.aborted) {
        return;
      }
      throw refusal(error);
    }
  }

  /**
   * Seals what was installed in an overlay workspace as the warm layer `name` of its
   * repository, for workspaces made later at the same commit to stand on. The layer is
   * moved, not copied: the workspace is removed.
   */
  async seal(path: string, name: string): Promise<WarmLayer> {
    const answer = await call(() => this.workspaces.seal({ path, name }));
    return toWarmLayer(answer.warm);
  }

  /** The warm layers of a repository. */
  async warmLayers(url: string): Promise<WarmLayer[]> {
    const answer = await call(() => this.workspaces.listWarm({ url }));
    return answer.warm.map(toWarmLayer);
  }

  /** Deletes a workspace; its pool stays. */
  async remove(path: string): Promise<void> {
    await call(() => this.workspaces.remove({ path }));
  }
}

/** The way to the gitd options name: its network address, else its socket, with the key. */
function transportFor(options: GitdOptions): Transport {
  const key = options.key ?? keyFromEnvironment();
  const interceptors: Interceptor[] = key
    ? [
        (next) => async (request) => {
          request.header.set("Authorization", `Bearer ${key}`);
          return next(request);
        },
      ]
    : [];
  const url = options.url ?? process.env.GITD_URL;
  if (url) {
    const caFile = process.env.GITD_CA_FILE;
    const ca = options.ca ?? (caFile ? readFileSync(caFile, "utf8") : undefined);
    return createConnectTransport({
      baseUrl: url.replace(/\/$/, ""),
      httpVersion: "1.1",
      interceptors,
      nodeOptions: ca ? { ca } : {},
    });
  }
  return createConnectTransport({
    baseUrl: "http://gitd",
    httpVersion: "1.1",
    interceptors,
    nodeOptions: { socketPath: options.socket ?? process.env.GITD_SOCKET ?? defaultSocket },
  });
}

function keyFromEnvironment(): string | undefined {
  const file = process.env.GITD_KEY_FILE;
  if (file) {
    return readFileSync(file, "utf8").trim();
  }
  return process.env.GITD_KEY?.trim() || undefined;
}
