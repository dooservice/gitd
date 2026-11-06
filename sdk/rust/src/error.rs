use std::io;
use std::path::PathBuf;

use tonic::{Code, Status};

use crate::proto::git_failure::Reason;
use crate::refusal::Refusal;

/// What went wrong: gitd refused the call, the call failed on its way, or the client could
/// not be made.
#[derive(Debug, thiserror::Error)]
pub enum Error {
    /// gitd refused the call, and said why.
    #[error("{0}")]
    Refused(Refusal),
    /// The call failed without a refusal of gitd's: gitd could not be reached, the key was
    /// refused, the call timed out.
    #[error("{} ({:?})", .0.message(), .0.code())]
    Call(Status),
    /// A streaming call ended without its result.
    #[error("gitd ended the call without a result")]
    NoResult,
    /// The network address is not one of a gitd.
    #[error("{0:?} is not the address of a gitd: https://host:port")]
    Address(String),
    /// The key holds what a header cannot.
    #[error("the key cannot be sent: it holds characters a header cannot")]
    Key,
    /// A file the options name could not be read.
    #[error("reading {}: {source}", path.display())]
    File {
        /// The file.
        path: PathBuf,
        /// Why it could not be read.
        source: io::Error,
    },
    /// The TLS configuration could not be applied.
    #[error("TLS: {0}")]
    Tls(tonic::transport::Error),
}

impl Error {
    /// gitd's reason for refusing the call, if it refused it.
    pub fn reason(&self) -> Option<Reason> {
        self.refusal().map(|refusal| refusal.reason)
    }

    /// gitd's refusal, if it refused the call.
    pub fn refusal(&self) -> Option<&Refusal> {
        match self {
            Self::Refused(refusal) => Some(refusal),
            _ => None,
        }
    }

    /// The call's status code, for a call that failed.
    pub fn code(&self) -> Option<Code> {
        match self {
            Self::Refused(refusal) => Some(refusal.code),
            Self::Call(status) => Some(status.code()),
            _ => None,
        }
    }
}

impl From<Status> for Error {
    fn from(status: Status) -> Self {
        match Refusal::of(&status) {
            Some(refusal) => Self::Refused(refusal),
            None => Self::Call(status),
        }
    }
}
