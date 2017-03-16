//! Provides the `Entry` type and implements its serialization methods.

use std::collections::BTreeMap;

// TODO: Can we enforce that keys are non-empty for directory maps? Maybe with a
// custom type. We'd also want to enforce that they don't contain invalid
// characters though. I think trying to enforce these constraints is a losing
// battle - the main win of moving to Rust is that we don't have mutations.
#[derive(Serialize, Deserialize)]
pub enum Entry {
    Directory(BTreeMap<String, Entry>),
    File{executable: bool, digest: Vec<u8>},
}