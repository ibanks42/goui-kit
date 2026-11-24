//! Little-endian binary reader ([`R`]) and writer ([`Put`]) for wire requests, replies, and tapes.
//!
//! Strings are encoded as a `u32` byte length followed by UTF-8 bytes; booleans use a single byte.

/// Cursor-based little-endian byte reader over an immutable wire buffer.
pub struct R<'a> {
    pub b: &'a [u8],
    pub i: usize,
}

impl<'a> R<'a> {
    /// Creates a reader positioned at byte offset 0 of `b`.
    pub fn new(b: &'a [u8]) -> Self {
        R { b, i: 0 }
    }

    /// Creates a reader positioned at byte offset `i` of `b`.
    pub fn at(b: &'a [u8], i: usize) -> Self {
        R { b, i }
    }

    #[inline]
    fn take(&mut self, n: usize) -> &'a [u8] {
        // Out-of-bounds panics are caught at the C ABI boundary and poison the runtime.
        let s = &self.b[self.i..self.i + n];
        self.i += n;
        s
    }

    #[inline]
    pub fn u8(&mut self) -> u8 {
        self.take(1)[0]
    }
    #[inline]
    pub fn bool(&mut self) -> bool {
        self.u8() != 0
    }
    #[inline]
    pub fn u16(&mut self) -> u16 {
        u16::from_le_bytes(self.take(2).try_into().unwrap())
    }
    #[inline]
    pub fn u32(&mut self) -> u32 {
        u32::from_le_bytes(self.take(4).try_into().unwrap())
    }
    #[inline]
    pub fn i32(&mut self) -> i32 {
        self.u32() as i32
    }
    #[inline]
    pub fn u64(&mut self) -> u64 {
        u64::from_le_bytes(self.take(8).try_into().unwrap())
    }
    #[inline]
    pub fn i64(&mut self) -> i64 {
        self.u64() as i64
    }
    #[inline]
    pub fn usize(&mut self) -> usize {
        self.u64() as usize
    }
    #[inline]
    pub fn f32(&mut self) -> f32 {
        f32::from_bits(self.u32())
    }
    #[inline]
    pub fn f64(&mut self) -> f64 {
        f64::from_bits(self.u64())
    }

    /// Reads a length-prefixed (`u32`) UTF-8 string slice, panicking on invalid UTF-8.
    #[inline]
    pub fn str(&mut self) -> &'a str {
        let n = self.u32() as usize;
        match std::str::from_utf8(self.take(n)) {
            Ok(s) => s,
            Err(e) => panic!(
                "a string on the wire is not UTF-8 (invalid byte at offset {} of {n}); the binding must send UTF-8",
                e.valid_up_to()
            ),
        }
    }

    /// Reads `n` raw bytes and advances the cursor.
    pub fn bytes(&mut self, n: usize) -> &'a [u8] {
        self.take(n)
    }

    /// Consumes and returns all remaining unread bytes.
    pub fn rest(&mut self) -> &'a [u8] {
        let s = &self.b[self.i..];
        self.i = self.b.len();
        s
    }

    /// Returns `true` when the cursor has reached the end of the buffer.
    pub fn done(&self) -> bool {
        self.i >= self.b.len()
    }
}

/// Append-only little-endian binary writer implemented for `Vec<u8>`.
pub trait Put {
    fn put_u8(&mut self, v: u8);
    fn put_bool(&mut self, v: bool);
    fn put_u16(&mut self, v: u16);
    fn put_u32(&mut self, v: u32);
    fn put_i32(&mut self, v: i32);
    fn put_u64(&mut self, v: u64);
    fn put_i64(&mut self, v: i64);
    fn put_f32(&mut self, v: f32);
    fn put_f64(&mut self, v: f64);
    fn put_str(&mut self, v: &str);
}

impl Put for Vec<u8> {
    fn put_u8(&mut self, v: u8) {
        self.push(v);
    }
    fn put_bool(&mut self, v: bool) {
        self.push(v as u8);
    }
    fn put_u16(&mut self, v: u16) {
        self.extend_from_slice(&v.to_le_bytes());
    }
    fn put_u32(&mut self, v: u32) {
        self.extend_from_slice(&v.to_le_bytes());
    }
    fn put_i32(&mut self, v: i32) {
        self.extend_from_slice(&v.to_le_bytes());
    }
    fn put_u64(&mut self, v: u64) {
        self.extend_from_slice(&v.to_le_bytes());
    }
    fn put_i64(&mut self, v: i64) {
        self.extend_from_slice(&v.to_le_bytes());
    }
    fn put_f32(&mut self, v: f32) {
        self.extend_from_slice(&v.to_bits().to_le_bytes());
    }
    fn put_f64(&mut self, v: f64) {
        self.extend_from_slice(&v.to_bits().to_le_bytes());
    }
    fn put_str(&mut self, v: &str) {
        self.put_u32(v.len() as u32);
        self.extend_from_slice(v.as_bytes());
    }
}
