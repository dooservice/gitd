use prost::Message as _;
use tonic::{Code, Status};

use crate::proto::GitFailure;
use crate::proto::git_failure::Reason;

/// What every reason's name starts with in the contract, and not in [`Refusal::reason_name`].
const REASON_PREFIX: &str = "REASON_";

/// Why gitd refused a call, from the `GitFailure` it attaches to it: the reason, to decide
/// on, and what git said, to show. Decide from the reason, never from the message.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Refusal {
    /// Why: `AuthRequired`, `NotFound`, `PathRefused`, `BranchRefused`, `Busy`, ….
    pub reason: Reason,
    /// The last useful line git wrote, e.g. "remote: Repository not found.".
    pub said: String,
    /// The repository the refusal is about, when gitd says.
    pub repository: String,
    /// The call's status code.
    pub code: Code,
}

impl Refusal {
    /// The refusal a failed call carries, if gitd attached one.
    pub(crate) fn of(status: &Status) -> Option<Self> {
        let details = tonic_types::Status::decode(status.details()).ok()?;
        let failure = details
            .details
            .iter()
            .find_map(|detail| detail.to_msg::<GitFailure>().ok())?;
        Some(Self {
            reason: failure.reason(),
            said: failure.git_said,
            repository: failure.repository,
            code: status.code(),
        })
    }

    /// The reason as the other clients and the command line name it: `AUTH_REQUIRED`,
    /// `NOT_FOUND`, …; empty when gitd gave none.
    pub fn reason_name(&self) -> &'static str {
        match self.reason {
            Reason::Unspecified => "",
            reason => reason.as_str_name().trim_start_matches(REASON_PREFIX),
        }
    }
}

impl std::fmt::Display for Refusal {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self.reason_name() {
            "" => formatter.write_str(&self.said),
            reason => write!(formatter, "{reason}: {}", self.said),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use prost_types::Any;

    fn refused(failure: &GitFailure) -> Status {
        let details = tonic_types::Status {
            code: Code::NotFound as i32,
            message: "not found".to_owned(),
            details: vec![Any::from_msg(failure).unwrap()],
        };
        Status::with_details(Code::NotFound, "not found", details.encode_to_vec().into())
    }

    #[test]
    fn reads_the_failure_gitd_attaches() {
        let failure = GitFailure {
            reason: Reason::NotFound as i32,
            git_said: "remote: Repository not found.".to_owned(),
            repository: "https://github.com/acme/gone".to_owned(),
        };
        let refusal = Refusal::of(&refused(&failure)).unwrap();
        assert_eq!(refusal.reason, Reason::NotFound);
        assert_eq!(refusal.reason_name(), "NOT_FOUND");
        assert_eq!(refusal.code, Code::NotFound);
        assert_eq!(
            refusal.to_string(),
            "NOT_FOUND: remote: Repository not found."
        );
    }

    #[test]
    fn finds_nothing_in_a_plain_status() {
        assert!(Refusal::of(&Status::unauthenticated("a valid key is needed")).is_none());
    }
}
