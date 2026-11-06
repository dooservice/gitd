use tonic::Streaming;

use crate::error::Error;
use crate::proto::{
    AdoptResponse, AdoptResult, CheckoutResponse, PrepareResponse, Progress,
    SyncSubmodulesResponse, SyncSubmodulesResult, UpdateResponse, WorkspaceResult, adopt_response,
    checkout_response, prepare_response, sync_submodules_response, update_response,
};

/// Told what git is doing while a long call runs.
pub type OnProgress = Box<dyn FnMut(&Progress) + Send>;

/// One message of a call that streams its progress and ends with its result.
pub(crate) enum Step<R> {
    Progress(Progress),
    Result(R),
    Empty,
}

/// A message of a call that streams its progress and ends with its result.
pub(crate) trait Stepping {
    type Result;

    fn step(self) -> Step<Self::Result>;
}

macro_rules! stepping {
    ($message:ty, $event:path, $result:ty) => {
        impl Stepping for $message {
            type Result = $result;

            fn step(self) -> Step<$result> {
                use $event as Event;
                match self.event {
                    Some(Event::Progress(progress)) => Step::Progress(progress),
                    Some(Event::Result(result)) => Step::Result(result),
                    None => Step::Empty,
                }
            }
        }
    };
}

stepping!(PrepareResponse, prepare_response::Event, WorkspaceResult);
stepping!(UpdateResponse, update_response::Event, WorkspaceResult);
stepping!(CheckoutResponse, checkout_response::Event, WorkspaceResult);
stepping!(
    SyncSubmodulesResponse,
    sync_submodules_response::Event,
    SyncSubmodulesResult
);
stepping!(AdoptResponse, adopt_response::Event, AdoptResult);

/// Reads a call that streams its progress to its result, telling the progress on the way.
pub(crate) struct Follow {
    on_progress: Option<OnProgress>,
}

impl Follow {
    pub(crate) fn new(on_progress: Option<OnProgress>) -> Self {
        Self { on_progress }
    }

    pub(crate) async fn result<M: Stepping>(
        mut self,
        mut stream: Streaming<M>,
    ) -> Result<M::Result, Error> {
        let mut result = None;
        while let Some(message) = stream.message().await? {
            match message.step() {
                Step::Progress(progress) => self.tell(&progress),
                Step::Result(last) => result = Some(last),
                Step::Empty => {}
            }
        }
        result.ok_or(Error::NoResult)
    }

    fn tell(&mut self, progress: &Progress) {
        if let Some(on_progress) = self.on_progress.as_mut() {
            on_progress(progress);
        }
    }
}
