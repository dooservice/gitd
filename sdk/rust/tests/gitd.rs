//! Calls a running gitd, the one the environment names as the command line reads it:
//! `GITD_URL`, `GITD_SOCKET`, `GITD_KEY_FILE`, `GITD_CA_FILE`. `GITD_TEST_ROOT` is one of its
//! roots, where a workspace of `GITD_TEST_REPOSITORY` (octocat/Hello-World by default) is
//! made and removed. Run with `cargo test -- --ignored`, as CI does (e2e/rust-sdk.sh).

use std::env;
use std::path::PathBuf;
use std::sync::Arc;
use std::sync::atomic::{AtomicUsize, Ordering};

use gitd::proto::ListTokensRequest;
use gitd::{DiffOptions, Error, Gitd, Options, PrepareOptions};
use tonic::Code;

const NEEDS_GITD: &str = "needs a running gitd; see e2e/rust-sdk.sh";
const DEFAULT_REPOSITORY: &str = "https://github.com/octocat/Hello-World";
const NOTE_FILE: &str = "AGENT.md";
const NOTE: &str = "Written by an agent.\n";

/// The gitd under test, and where it makes workspaces.
struct Subject {
    gitd: Gitd,
    root: PathBuf,
    repository: String,
}

impl Subject {
    async fn new() -> Self {
        Self {
            gitd: Gitd::connect(Options::new())
                .await
                .expect("the options are valid"),
            root: PathBuf::from(
                env::var("GITD_TEST_ROOT").expect("GITD_TEST_ROOT names a root of gitd's"),
            ),
            repository: env::var("GITD_TEST_REPOSITORY")
                .unwrap_or_else(|_| DEFAULT_REPOSITORY.to_owned()),
        }
    }

    fn workspace(&self, name: &str) -> String {
        self.root
            .join(format!("{name}-{}", std::process::id()))
            .to_string_lossy()
            .into_owned()
    }
}

#[tokio::test]
#[ignore = "needs a running gitd; see e2e/rust-sdk.sh"]
async fn answers_which_gitd_it_is() {
    let subject = Subject::new().await;
    let info = subject.gitd.info().await.expect(NEEDS_GITD);
    assert!(!info.version.is_empty());
    let tokens = subject
        .gitd
        .endpoint()
        .list_tokens(ListTokensRequest {})
        .await
        .expect("tokens are listed");
    println!(
        "reached gitd {} with {} tokens",
        info.version,
        tokens.into_inner().tokens.len()
    );
}

#[tokio::test]
#[ignore = "needs a running gitd; see e2e/rust-sdk.sh"]
async fn refuses_a_wrong_key_on_the_network() {
    if env::var("GITD_URL").is_err() {
        println!("on the socket, where no key is asked for");
        return;
    }
    let gitd = Gitd::connect(Options::new().key("wrong"))
        .await
        .expect("the options are valid");
    let refused = gitd.info().await.expect_err("a wrong key was answered");
    assert_eq!(refused.code(), Some(Code::Unauthenticated), "{refused}");
}

#[tokio::test]
#[ignore = "needs a running gitd; see e2e/rust-sdk.sh"]
async fn makes_reads_and_removes_a_workspace() {
    let subject = Subject::new().await;
    let path = subject.workspace("rust-sdk");
    let steps = Arc::new(AtomicUsize::new(0));
    let counted = Arc::clone(&steps);
    let options = PrepareOptions {
        on_progress: Some(Box::new(move |_| {
            counted.fetch_add(1, Ordering::Relaxed);
        })),
        ..PrepareOptions::default()
    };
    let workspace = subject
        .gitd
        .prepare(&subject.repository, &path, options)
        .await
        .expect("prepared");
    println!(
        "prepared at {} by {:?}, {} progress steps",
        workspace.commit,
        workspace.strategy(),
        steps.load(Ordering::Relaxed)
    );

    std::fs::write(PathBuf::from(&path).join(NOTE_FILE), NOTE).expect("written");
    let described = subject.gitd.describe(&path).await.expect("described");
    assert_eq!(described.commit, workspace.commit);
    let changes = subject
        .gitd
        .changes(&path, false)
        .await
        .expect("changes read");
    assert!(
        changes.files.iter().any(|file| file.path == NOTE_FILE),
        "{changes:?}"
    );
    let diff = subject
        .gitd
        .diff(&path, DiffOptions::default())
        .await
        .expect("diff read");
    assert!(diff.diff.contains(NOTE.trim_end()), "{}", diff.diff);

    subject.gitd.remove(&path).await.expect("removed");
    let refused = subject
        .gitd
        .describe(&path)
        .await
        .expect_err("a removed workspace was described");
    assert!(matches!(refused, Error::Refused(_)), "{refused:?}");
    println!("a removed workspace is refused: {refused}");
}
