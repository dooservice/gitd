use std::future::Future;
use std::pin::Pin;
use std::task::{Context, Poll};

use http::{Request, Response, StatusCode};
use tonic::{Code, Status};
use tower::Service;

/// The header every gRPC answer carries, in its headers or trailers.
const GRPC_STATUS: &str = "grpc-status";

type BoxError = Box<dyn std::error::Error + Send + Sync>;

/// Reads a plain HTTP error that gitd answers a call with — as its network address answers a
/// missing or wrong key with `401 Unauthorized` — as the gRPC status it stands for, by
/// gRPC's own mapping, instead of as a malformed answer.
#[derive(Clone, Debug)]
pub struct HttpRefusal<S> {
    inner: S,
}

impl<S> HttpRefusal<S> {
    pub(crate) fn new(inner: S) -> Self {
        Self { inner }
    }

    /// The status a plain HTTP answer stands for, as gRPC maps them; none for a gRPC answer.
    fn status_of<B>(response: &Response<B>) -> Option<Status> {
        let answered = response.status();
        if answered == StatusCode::OK || response.headers().contains_key(GRPC_STATUS) {
            return None;
        }
        let code = match answered {
            StatusCode::BAD_REQUEST => Code::Internal,
            StatusCode::UNAUTHORIZED => Code::Unauthenticated,
            StatusCode::FORBIDDEN => Code::PermissionDenied,
            StatusCode::NOT_FOUND => Code::Unimplemented,
            StatusCode::TOO_MANY_REQUESTS
            | StatusCode::BAD_GATEWAY
            | StatusCode::SERVICE_UNAVAILABLE
            | StatusCode::GATEWAY_TIMEOUT => Code::Unavailable,
            _ => Code::Unknown,
        };
        Some(Status::new(code, format!("gitd answered HTTP {answered}")))
    }
}

impl<S, B, R> Service<Request<B>> for HttpRefusal<S>
where
    S: Service<Request<B>, Response = Response<R>>,
    S::Error: Into<BoxError>,
    S::Future: Send + 'static,
{
    type Response = Response<R>;
    type Error = BoxError;
    type Future = Pin<Box<dyn Future<Output = Result<Response<R>, BoxError>> + Send>>;

    fn poll_ready(&mut self, context: &mut Context<'_>) -> Poll<Result<(), BoxError>> {
        self.inner.poll_ready(context).map_err(Into::into)
    }

    fn call(&mut self, request: Request<B>) -> Self::Future {
        let answer = self.inner.call(request);
        Box::pin(async move {
            let response = answer.await.map_err(Into::into)?;
            match Self::status_of(&response) {
                Some(status) => Err(status.into()),
                None => Ok(response),
            }
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn answered(status: StatusCode) -> Response<()> {
        Response::builder().status(status).body(()).unwrap()
    }

    #[test]
    fn reads_a_refused_key_as_unauthenticated() {
        let status = HttpRefusal::<()>::status_of(&answered(StatusCode::UNAUTHORIZED)).unwrap();
        assert_eq!(status.code(), Code::Unauthenticated);
    }

    #[test]
    fn leaves_grpc_answers_alone() {
        assert!(HttpRefusal::<()>::status_of(&answered(StatusCode::OK)).is_none());
        let mut refused = answered(StatusCode::UNAUTHORIZED);
        refused
            .headers_mut()
            .insert(GRPC_STATUS, "16".parse().unwrap());
        assert!(HttpRefusal::<()>::status_of(&refused).is_none());
    }
}
