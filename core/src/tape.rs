//! The tape (DESIGN.md section 6): what a view procedure wrote, kept by the view entity and
//! decoded into real GPUI-kit elements when GPUI builds that view.
//!
//! Tape bytes, as the binding writes them:
//!   u32 magic "GKT1", u32 node_count, u32 root, u32 handler_count,
//!   u16 n_deps, u64 * n      kit state the view procedure read
//!   u16 n_slots, u64 * n     child views the tape embeds
//!   u32 ops_len, ops         the op stream; offsets are relative to its start, 0 = none
//!   nodes: node_count * (u16 type, u32 args, u32 first_op)
//! An op is u16 opcode, u32 offset of the next op of the same node, arguments.

use crate::ops::*;
use crate::rt::{GkErr, GkResult};
use crate::wire::R;

const MAGIC: u32 = 0x3154_4b47;
const NODE_SIZE: usize = 10;

pub struct Tape {
    bytes: Vec<u8>,
    pub generation: u32,
    ops_at: usize,
    ops_len: usize,
    nodes_at: usize,
    node_count: u32,
    pub root: u32,
    pub deps: Vec<u64>,
    pub slots: Vec<u64>,
}

impl Tape {
    pub fn parse(bytes: Vec<u8>, generation: u32) -> GkResult<Tape> {
        let bad = |m: &str| GkErr(ERR_BAD_TAPE, m.to_string());
        if bytes.len() < 28 {
            return Err(bad("tape shorter than its header"));
        }
        let mut r = R::new(&bytes);
        if r.u32() != MAGIC {
            return Err(bad("bad tape magic"));
        }
        let node_count = r.u32();
        let root = r.u32();
        let _handlers = r.u32();
        let n_deps = r.u16() as usize;
        if bytes.len() < r.i + n_deps * 8 + 2 {
            return Err(bad("tape cut in its dependency list"));
        }
        let deps = (0..n_deps).map(|_| r.u64()).collect();
        let n_slots = r.u16() as usize;
        if bytes.len() < r.i + n_slots * 8 + 4 {
            return Err(bad("tape cut in its slot list"));
        }
        let slots = (0..n_slots).map(|_| r.u64()).collect();
        let ops_len = r.u32() as usize;
        let ops_at = r.i;
        let nodes_at = ops_at + ops_len;
        if bytes.len() != nodes_at + node_count as usize * NODE_SIZE || root >= node_count {
            return Err(bad("tape length does not match its node table"));
        }
        Ok(Tape {
            bytes,
            generation,
            ops_at,
            ops_len,
            nodes_at,
            node_count,
            root,
            deps,
            slots,
        })
    }

    /// (type, args offset, first op offset) of a node.
    pub fn node(&self, n: u32) -> (u16, u32, u32) {
        assert!(n < self.node_count, "gk: tape names node {n} of {}", self.node_count);
        let mut r = R::at(&self.bytes, self.nodes_at + n as usize * NODE_SIZE);
        (r.u16(), r.u32(), r.u32())
    }

    pub fn ops(&self) -> &[u8] {
        &self.bytes[self.ops_at..self.ops_at + self.ops_len]
    }
}
