//! A client for [gitd](https://github.com/dooservice/gitd): instant, secretless git
//! checkouts for agent sandboxes.
#![warn(missing_docs)]

mod error;
pub mod proto;
mod refusal;

pub use error::Error;
pub use proto::git_failure::Reason;
pub use refusal::Refusal;
