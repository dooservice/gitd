use std::time::SystemTime;

/// Which part of a workspace's diff [`Gitd::diff`](crate::Gitd::diff) reads.
#[derive(Clone, Debug, Default)]
pub struct DiffOptions {
    /// Only these files, relative to the workspace; by default all.
    pub files: Vec<String>,
    /// At most this many bytes: 512 KiB when zero, never more than 8 MiB.
    pub max_bytes: i64,
    /// The files git ignores too.
    pub include_ignored: bool,
}

/// What [`Gitd::compare`](crate::Gitd::compare) reads of two works.
#[derive(Clone, Debug, Default)]
pub struct CompareOptions {
    /// The repository, when both sides are branches.
    pub url: String,
    /// The unified diff too, besides the files.
    pub with_diff: bool,
    /// At most this many bytes of diff: 512 KiB when zero.
    pub max_bytes: i64,
    /// The files git ignores too.
    pub include_ignored: bool,
}

/// Which entries of the audit log [`Gitd::audit`](crate::Gitd::audit) reads.
#[derive(Clone, Debug, Default)]
pub struct AuditOptions {
    /// Only those from this time on.
    pub since: Option<SystemTime>,
    /// Only those of this repository, by its URL.
    pub url: String,
    /// At most this many: 100 when zero, never more than 1000.
    pub limit: i32,
}
