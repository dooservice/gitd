//! A client for [gitd](https://github.com/dooservice/gitd): instant, secretless git
//! checkouts for agent sandboxes.
#![warn(missing_docs)]

mod client;
mod environment;
mod error;
mod http_refusal;
mod key;
mod options;
pub mod proto;
mod refusal;
mod review_options;
mod socket;
mod target;

pub use client::{Gitd, Transport};
pub use environment::{
    CA_FILE_VARIABLE, KEY_FILE_VARIABLE, KEY_VARIABLE, SOCKET_VARIABLE, URL_VARIABLE,
};
pub use error::Error;
pub use http_refusal::HttpRefusal;
pub use key::Key;
pub use options::Options;
pub use proto::git_failure::Reason;
pub use refusal::Refusal;
pub use review_options::{AuditOptions, CompareOptions, DiffOptions};
pub use target::DEFAULT_SOCKET;
