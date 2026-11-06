use tonic::metadata::{Ascii, MetadataValue};
use tonic::service::Interceptor;
use tonic::{Request, Status};

use crate::error::Error;

/// The header a caller brings its key in, as gitd's network address asks for it.
const HEADER: &str = "authorization";
/// What comes before the key in [`HEADER`].
const SCHEME: &str = "Bearer ";

/// The key naming the caller's tenant, brought on every call as
/// `authorization: Bearer <key>`; without a key, calls go as they are, as on a gitd's socket.
#[derive(Clone, Default)]
pub struct Key {
    header: Option<MetadataValue<Ascii>>,
}

impl Key {
    /// The key, or none when it is empty; refused when it cannot travel in a header.
    pub(crate) fn new(key: Option<String>) -> Result<Self, Error> {
        let Some(key) = key.filter(|key| !key.is_empty()) else {
            return Ok(Self::default());
        };
        let mut header =
            MetadataValue::try_from(format!("{SCHEME}{key}")).map_err(|_| Error::Key)?;
        header.set_sensitive(true);
        Ok(Self {
            header: Some(header),
        })
    }
}

impl Interceptor for Key {
    fn call(&mut self, mut request: Request<()>) -> Result<Request<()>, Status> {
        if let Some(header) = &self.header {
            request.metadata_mut().insert(HEADER, header.clone());
        }
        Ok(request)
    }
}

impl std::fmt::Debug for Key {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("Key")
            .field("present", &self.header.is_some())
            .finish()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn brings_the_key_as_a_bearer() {
        let mut key = Key::new(Some("s3cret".to_owned())).unwrap();
        let request = key.call(Request::new(())).unwrap();
        assert_eq!(request.metadata().get(HEADER).unwrap(), "Bearer s3cret");
        assert!(!format!("{key:?}").contains("s3cret"));
    }

    #[test]
    fn brings_nothing_without_a_key() {
        let mut key = Key::new(Some(String::new())).unwrap();
        assert!(
            key.call(Request::new(()))
                .unwrap()
                .metadata()
                .get(HEADER)
                .is_none()
        );
    }

    #[test]
    fn refuses_a_key_a_header_cannot_hold() {
        assert!(matches!(
            Key::new(Some("two\nlines".to_owned())),
            Err(Error::Key)
        ));
    }
}
