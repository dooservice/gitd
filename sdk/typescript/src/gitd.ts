import { createClient, type Client, type Transport } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-node";

import { RepositoryService, ServerService, WorkspaceService } from "./gen/gitd/v1/gitd_pb.js";
import { call, drain } from "./calls.js";
import { accessName, revisionOf, strategyName, strategyProto, toWorkspace } from "./convert.js";
import type {
  CheckoutOptions,
  Description,
  Info,
  PrepareOptions,
  Probe,
  Published,
  UpdateOptions,
  Workspace,
} from "./types.js";

export interface GitdOptions {
  /** gitd's Unix socket; default $GITD_SOCKET, then /run/gitd/gitd.sock. */
  socket?: string;
  /** A transport of your own instead, e.g. for tests. */
  transport?: Transport;
}

const defaultSocket = "/run/gitd/gitd.sock";

/**
 * A gitd on its Unix socket. No call takes a credential: gitd reads and publishes with the
 * ones it keeps.
 */
export class Gitd {
  private readonly workspaces: Client<typeof WorkspaceService>;
  private readonly repositories: Client<typeof RepositoryService>;
  private readonly server: Client<typeof ServerService>;

  constructor(options: GitdOptions = {}) {
    const transport = options.transport ?? socketTransport(options.socket);
    this.workspaces = createClient(WorkspaceService, transport);
    this.repositories = createClient(RepositoryService, transport);
    this.server = createClient(ServerService, transport);
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
    };
  }

  /** Pushes what was committed in a workspace to a new branch, with the token gitd keeps. */
  async publish(path: string, branch: string): Promise<Published> {
    const answer = await call(() => this.workspaces.publish({ path, branch }));
    const pushed = [answer.repository, ...answer.submodules].filter((repository) => repository !== undefined);
    return {
      branch: answer.branch,
      repositories: pushed.map((repository) => ({
        url: repository.url,
        path: repository.path,
        commit: repository.commit,
        subjects: repository.subjects,
      })),
    };
  }

  /** Deletes a workspace; its pool stays. */
  async remove(path: string): Promise<void> {
    await call(() => this.workspaces.remove({ path }));
  }
}

function socketTransport(socket?: string): Transport {
  return createConnectTransport({
    baseUrl: "http://gitd",
    httpVersion: "1.1",
    nodeOptions: { socketPath: socket ?? process.env.GITD_SOCKET ?? defaultSocket },
  });
}
