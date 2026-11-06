//! A client for [gitd](https://github.com/dooservice/gitd): instant, secretless git
//! checkouts for agent sandboxes. Agents work in their own copy and publish a branch; they
//! never see a token.
//!
//! ```no_run
//! use gitd::{Gitd, Options, PrepareOptions, PublishOptions};
//!
//! # async fn run() -> Result<(), gitd::Error> {
//! let gitd = Gitd::connect(Options::new()).await?; // $GITD_SOCKET, then the default socket
//! let path = "/var/lib/gitd/workspaces/agent-1";
//! let workspace = gitd.prepare("https://github.com/acme/web", path, PrepareOptions::default()).await?;
//! println!("{} at {}", path, workspace.commit);
//!
//! // … the agent works in the workspace with plain git, and commits …
//!
//! gitd.publish(path, "agents/fix-login", PublishOptions::default()).await?;
//! gitd.remove(path).await?;
//! # Ok(())
//! # }
//! ```
//!
//! What gitd answers is the contract's own messages, in [`proto`]. A refusal is an
//! [`Error::Refused`], whose [`Refusal`] has the reason to decide on and what git said.
#![warn(missing_docs)]

mod client;
mod environment;
mod error;
mod follow;
mod http_refusal;
mod key;
mod options;
pub mod proto;
mod publish_options;
mod refusal;
mod review;
mod review_options;
mod side;
mod socket;
mod target;
mod workspace_options;
mod workspaces;

pub use client::{Gitd, Transport};
pub use environment::{
    CA_FILE_VARIABLE, KEY_FILE_VARIABLE, KEY_VARIABLE, SOCKET_VARIABLE, URL_VARIABLE,
};
pub use error::Error;
pub use follow::OnProgress;
pub use http_refusal::HttpRefusal;
pub use key::Key;
pub use options::Options;
pub use proto::git_failure::Reason;
pub use proto::{Progress, Side, Strategy};
pub use publish_options::PublishOptions;
pub use refusal::Refusal;
pub use review_options::{AuditOptions, CompareOptions, DiffOptions};
pub use target::DEFAULT_SOCKET;
pub use workspace_options::{CheckoutOptions, PrepareOptions, UpdateOptions};
