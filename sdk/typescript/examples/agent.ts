/**
 * An agent's round trip with gitd:
 *
 *   1. gitd prepares a workspace of the repository, in milliseconds after the first time;
 *   2. the agent works in it with plain git, and commits;
 *   3. gitd publishes the commit as a new branch, with the token it keeps.
 *
 * The agent never holds a credential.
 *
 *   npm run example -- https://github.com/your-org/your-repo /var/lib/gitd/workspaces/agent-1
 */
import { execFileSync } from "node:child_process";
import { writeFileSync } from "node:fs";
import { join } from "node:path";

import { Gitd, GitdError, type Workspace } from "gitd-client";

const gitd = new Gitd();

async function main(url: string, path: string): Promise<void> {
  try {
    const workspace = await prepare(url, path);
    commitAChange(workspace);
    await publish(workspace);
  } catch (error) {
    if (!(error instanceof GitdError)) throw error;
    console.error(`gitd refused: ${error.reason}: ${error.said}`);
    process.exitCode = 1;
  } finally {
    await gitd.remove(path).catch(() => {});
  }
}

async function prepare(url: string, path: string): Promise<Workspace> {
  const workspace = await gitd.prepare({
    url,
    path,
    onProgress: (progress) => process.stderr.write(`\r${progress.phase} ${progress.percent}%`),
  });
  process.stderr.write("\n");
  console.log(`Prepared ${workspace.path} at ${workspace.commit.slice(0, 12)}, by ${workspace.strategy}.`);
  return workspace;
}

function commitAChange(workspace: Workspace): void {
  writeFileSync(join(workspace.path, "AGENT.md"), "Written by an agent.\n");
  git(workspace, "add", "AGENT.md");
  git(workspace, "commit", "--quiet", "--message", "An agent was here");
  console.log("Committed AGENT.md.");
}

async function publish(workspace: Workspace): Promise<void> {
  const branch = `agents/example-${Date.now()}`;
  const published = await gitd.publish(workspace.path, branch);
  const [repository] = published.repositories;
  console.log(`Published ${published.branch} to ${repository.url}: ${repository.subjects.join(", ")}.`);
}

function git(workspace: Workspace, ...args: string[]): void {
  const author = ["-c", "user.name=agent", "-c", "user.email=agent@example.com"];
  execFileSync("git", [...author, ...args], { cwd: workspace.path, stdio: "inherit" });
}

const [url, path] = process.argv.slice(2);
if (!url || !path) {
  console.error("usage: npm run example -- <repository url> <workspace path>");
  process.exit(2);
}
await main(url, path);
