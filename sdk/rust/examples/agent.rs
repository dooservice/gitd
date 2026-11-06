//! An agent's round trip with gitd:
//!
//!   1. gitd prepares a workspace of the repository, in milliseconds after the first time;
//!   2. the agent works in it with plain git, and commits;
//!   3. gitd publishes the commit as a new branch, with the token it keeps.
//!
//! The agent never holds a credential.
//!
//! ```text
//! cargo run --example agent -- https://github.com/your-org/your-repo /var/lib/gitd/workspaces/agent-1
//! ```

use std::io::Write as _;
use std::path::Path;
use std::process::{Command, ExitCode};
use std::time::{SystemTime, UNIX_EPOCH};
use std::{env, fs, io};

use gitd::{Error, Gitd, Options, PrepareOptions, Progress, PublishOptions};

const AUTHOR: [&str; 4] = [
    "-c",
    "user.name=agent",
    "-c",
    "user.email=agent@example.com",
];
const BRANCH_PREFIX: &str = "agents/example-";
const NOTE_FILE: &str = "AGENT.md";
const USAGE: &str = "usage: cargo run --example agent -- <repository url> <workspace path>";

/// Works on one repository in its own workspace, and publishes what it commits.
struct Agent {
    gitd: Gitd,
    url: String,
    path: String,
}

impl Agent {
    async fn run(&self) -> Result<(), Error> {
        self.prepare().await?;
        let published = match self.commit_a_change() {
            Ok(()) => self.publish().await,
            Err(failed) => {
                eprintln!("git failed: {failed}");
                Ok(())
            }
        };
        self.gitd.remove(&self.path).await?;
        published
    }

    async fn prepare(&self) -> Result<(), Error> {
        let options = PrepareOptions {
            on_progress: Some(Box::new(Self::show_progress)),
            ..PrepareOptions::default()
        };
        let workspace = self.gitd.prepare(&self.url, &self.path, options).await?;
        eprintln!();
        let short = workspace.commit.get(..12).unwrap_or(&workspace.commit);
        println!(
            "Prepared {} at {short}, by {:?}.",
            self.path,
            workspace.strategy()
        );
        Ok(())
    }

    fn commit_a_change(&self) -> io::Result<()> {
        fs::write(
            Path::new(&self.path).join(NOTE_FILE),
            "Written by an agent.\n",
        )?;
        self.git(&["add", NOTE_FILE])?;
        self.git(&["commit", "--quiet", "--message", "An agent was here"])?;
        println!("Committed {NOTE_FILE}.");
        Ok(())
    }

    async fn publish(&self) -> Result<(), Error> {
        let seconds = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .map_or(0, |since| since.as_secs());
        let branch = format!("{BRANCH_PREFIX}{seconds}");
        let published = self
            .gitd
            .publish(&self.path, branch, PublishOptions::default())
            .await?;
        if let Some(repository) = published.repository {
            println!(
                "Published {} to {}: {}.",
                published.branch,
                repository.url,
                repository.subjects.join(", ")
            );
        }
        Ok(())
    }

    fn git(&self, arguments: &[&str]) -> io::Result<()> {
        let status = Command::new("git")
            .args(AUTHOR)
            .args(arguments)
            .current_dir(&self.path)
            .status()?;
        if status.success() {
            Ok(())
        } else {
            Err(io::Error::other(format!("git {} {status}", arguments[0])))
        }
    }

    fn show_progress(progress: &Progress) {
        eprint!("\r{} {}%", progress.phase, progress.percent);
        let _ = io::stderr().flush();
    }
}

#[tokio::main]
async fn main() -> ExitCode {
    let arguments: Vec<String> = env::args().skip(1).collect();
    let [url, path] = arguments.as_slice() else {
        eprintln!("{USAGE}");
        return ExitCode::from(2);
    };
    let ran = match Gitd::connect(Options::new()).await {
        Ok(gitd) => {
            Agent {
                gitd,
                url: url.clone(),
                path: path.clone(),
            }
            .run()
            .await
        }
        Err(failed) => Err(failed),
    };
    match ran {
        Ok(()) => ExitCode::SUCCESS,
        Err(Error::Refused(refusal)) => {
            eprintln!("gitd refused: {refusal}");
            ExitCode::FAILURE
        }
        Err(failed) => {
            eprintln!("gitd: {failed}");
            ExitCode::FAILURE
        }
    }
}
